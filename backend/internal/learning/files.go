package learning

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const imageLimit = 8 << 20
const pdfLimit = 20 << 20

type CorrectionProgress struct {
	Stage     string    `json:"stage"`
	Attempts  int       `json:"attempts"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Submission struct {
	ReviewVideoURL     string             `json:"reviewVideoUrl,omitempty"`
	SupportAccess      bool               `json:"supportAccess,omitempty"`
	CorrectionProgress CorrectionProgress `json:"correctionProgress"`
	Correction         json.RawMessage    `json:"correction,omitempty"`
	ID                 string             `json:"id"`
	ExerciseID         string             `json:"exerciseId"`
	Status             string             `json:"status"`
	Feedback           string             `json:"feedback"`
	Text               string             `json:"text"`
	HasImage           bool               `json:"hasImage"`
	CreatedAt          time.Time          `json:"createdAt"`
}
type Document struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Size  int64  `json:"size"`
}

const submissionBase = "id::text,exercise_id::text,status,feedback,text_answer,object_key<>'',created_at,correction,correction_progress"

// The SQL projection hides private feedback links before serializing any member,
// admin, or offline retry response.
func submissionFields(plans string) string {
	eligible := "EXISTS(SELECT FROM member_rights e WHERE e.user_id=submission_tracking.user_id AND e.valid_until>now() AND NOT e.refunded AND (e.tier='suivi' OR (e.source NOT IN ('apple','google') AND e.plan_key=ANY(" + plans + "::text[]))))"
	return submissionBase + "," + eligible + ",CASE WHEN " + eligible + " THEN coalesce((SELECT review_video_url FROM submission_video_reviews WHERE submission_id=submission_tracking.id),'') ELSE '' END"
}
func scanSubmission(row pgx.Row) (Submission, error) {
	var s Submission
	err := row.Scan(&s.ID, &s.ExerciseID, &s.Status, &s.Feedback, &s.Text, &s.HasImage, &s.CreatedAt, &s.Correction, &s.CorrectionProgress, &s.SupportAccess, &s.ReviewVideoURL)
	return s, err
}
func multipartFields(w http.ResponseWriter, r *http.Request, limits map[string]int, total int) (map[string][]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(total+64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, problem{415, "Formulaire multipart attendu."}
	}
	fields := map[string][]byte{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				return nil, problem{413, "Fichier trop volumineux."}
			}
			return nil, bad("Formulaire illisible.")
		}
		name := part.FormName()
		limit, ok := limits[name]
		if _, duplicate := fields[name]; !ok || duplicate {
			return nil, bad("Champ inattendu ou dupliqué.")
		}
		data, err := io.ReadAll(io.LimitReader(part, int64(limit+1)))
		if err != nil {
			return nil, bad("Fichier incomplet.")
		}
		if len(data) > limit {
			return nil, problem{413, "Fichier trop volumineux."}
		}
		fields[name] = data
	}
	return fields, nil
}

// Parse and re-encode uploads to discard embedded metadata and trailing data.
func cleanImage(data []byte) ([]byte, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, "", problem{415, "Utilisez une image JPEG ou PNG."}
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return nil, "", problem{413, "Dimensions de l’image trop grandes."}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", bad("Image incomplète ou endommagée.")
	}
	var b bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(&b, img)
	}
	if err != nil {
		return nil, "", bad("Image illisible.")
	}
	if b.Len() > imageLimit {
		return nil, "", problem{413, "Image trop volumineuse."}
	}
	return b.Bytes(), "image/" + format, nil
}

// Reserve cleanup before PUT. A crashed upload is removed after the grace period.
func (a *App) stage(ctx context.Context, prefix, mime string, body []byte) (string, error) {
	if a.store == nil {
		return "", unavailable()
	}
	key := prefix + "/" + token()
	_, err := a.db.Exec(ctx, `INSERT INTO jobs(kind,payload,deduplication_key,available_at) VALUES('delete_object',jsonb_build_object('key',$1::text),'stage/'||$1,now()+interval '1 hour')`, key)
	if err != nil {
		return "", err
	}
	if err = a.store.Put(ctx, key, bytes.NewReader(body), int64(len(body)), mime); err != nil {
		return "", unavailable()
	}
	return key, nil
}
func attach(ctx context.Context, tx pgx.Tx, key string) error {
	if key == "" {
		return nil
	}
	tag, err := tx.Exec(ctx, "DELETE FROM jobs WHERE deduplication_key='stage/'||$1 AND status='pending'", key)
	if err == nil && tag.RowsAffected() != 1 {
		return problem{409, "Dépôt expiré, recommencez."}
	}
	return err
}
func (a *App) submit(w http.ResponseWriter, r *http.Request, u User) error {
	ctx := r.Context()
	f, err := multipartFields(w, r, map[string]int{"image": imageLimit, "text": 48_000, "exerciseId": 36, "clientId": 36}, imageLimit+48_100)
	if err != nil {
		return err
	}
	eid, client, answer := string(f["exerciseId"]), string(f["clientId"]), strings.TrimSpace(string(f["text"]))
	if !uuid(eid) || client != "" && !uuid(client) || !utf8.ValidString(answer) || utf8.RuneCountInString(answer) > 12000 || answer == "" && len(f["image"]) == 0 {
		return bad("Copie ou exercice invalide.")
	}
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s/%d/", eid, len(answer)) // hash.Hash writes never fail.
	h.Write([]byte(answer))
	h.Write(f["image"])
	digest := h.Sum(nil)
	lookup := func(q rowQuery) (Submission, error) {
		var match bool
		var s Submission
		err := q.QueryRow(ctx, "SELECT "+submissionFields("$4")+",request_hash=$3 FROM submission_tracking WHERE user_id=$1 AND client_id=$2", u.ID, client, digest, a.opts.Billing.SupportPlans()).Scan(&s.ID, &s.ExerciseID, &s.Status, &s.Feedback, &s.Text, &s.HasImage, &s.CreatedAt, &s.Correction, &s.CorrectionProgress, &s.SupportAccess, &s.ReviewVideoURL, &match)
		if err == nil && !match {
			return s, problem{409, "Identifiant de copie déjà utilisé."}
		}
		return s, err
	}
	if client != "" {
		s, err := lookup(a.db)
		if err == nil {
			return jsonOut(w, 201, s)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if err = a.rate(ctx, "submissions/"+u.ID, 20); err != nil {
		return err
	}
	var exists bool
	if err = a.db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM published_content c WHERE id=$1 AND kind='exercise' AND NOT EXISTS (SELECT FROM content p WHERE p.id=c.parent_id AND p.kind='mock'))", eid).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return missing()
	}
	var key, mime string
	if len(f["image"]) > 0 {
		data, m, e := cleanImage(f["image"])
		if e != nil {
			return e
		}
		mime = m
		key, e = a.stage(ctx, "submissions", mime, data)
		if e != nil {
			return e
		}
	}
	var out Submission
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		if client != "" {
			if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,1907))", u.ID+client); e != nil {
				return e
			}
			s, e := lookup(tx)
			if e == nil {
				out = s
				return nil
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
		}
		var e error
		var id string
		e = tx.QueryRow(ctx, `INSERT INTO submissions(user_id,exercise_id,text_answer,object_key,content_type,client_id,request_hash)
SELECT $1,id,$3,$4,$5,nullif($6,'')::uuid,$7 FROM published_content WHERE id=$2 RETURNING id::text`, u.ID, eid, answer, key, mime, client, digest).Scan(&id)
		if e != nil {
			return e
		}
		if e = attach(ctx, tx, key); e != nil {
			return e
		}
		if a.opts.CorrectionEnabled {
			_, e = tx.Exec(ctx, `INSERT INTO jobs(kind,payload,deduplication_key) VALUES('correct_submission',jsonb_build_object('submissionId',$1::text),'correction/'||$1)`, id)
		}
		if e != nil {
			return e
		}
		out, e = scanSubmission(tx.QueryRow(ctx, "SELECT "+submissionFields("$2")+" FROM submission_tracking WHERE id=$1", id, a.opts.Billing.SupportPlans()))
		return e
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 201, out)
}

type rowQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (a *App) submissions(w http.ResponseWriter, r *http.Request, u User) error {
	admin := strings.HasPrefix(r.URL.Path, "/v1/admin/")
	rows, err := a.db.Query(r.Context(), "SELECT "+submissionFields("$3")+" FROM submission_tracking WHERE $2::boolean OR user_id=$1 ORDER BY created_at DESC,id", u.ID, admin, a.opts.Billing.SupportPlans())
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Submission, error) { return scanSubmission(row) })
	if err != nil {
		return err
	}
	return jsonOut(w, 200, map[string]any{"items": items})
}
func (a *App) review(w http.ResponseWriter, r *http.Request, _ User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Feedback       string  `json:"feedback"`
		ReviewVideoURL *string `json:"reviewVideoUrl"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	in.Feedback = strings.TrimSpace(in.Feedback)
	if in.Feedback == "" && (in.ReviewVideoURL == nil || *in.ReviewVideoURL == "") || len(in.Feedback) > 32000 || in.ReviewVideoURL != nil && !safeLink(*in.ReviewVideoURL) {
		return bad("Commentaire invalide.")
	}
	ctx := r.Context()
	var out Submission
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, "UPDATE submissions SET status='reviewed',feedback=$2,correction=NULL WHERE id=$1", id, in.Feedback)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return missing()
		}
		if in.ReviewVideoURL != nil {
			if *in.ReviewVideoURL == "" {
				if _, err := tx.Exec(ctx, "DELETE FROM submission_video_reviews WHERE submission_id=$1", id); err != nil {
					return err
				}
			} else {
				tag, err := tx.Exec(ctx, `INSERT INTO submission_video_reviews(submission_id,review_video_url)
 SELECT s.id,$2 FROM submissions s WHERE s.id=$1 AND EXISTS(SELECT FROM member_rights e WHERE e.user_id=s.user_id AND e.valid_until>now() AND NOT e.refunded AND (e.tier='suivi' OR (e.source NOT IN ('apple','google') AND e.plan_key=ANY($3::text[]))))
 ON CONFLICT(submission_id) DO UPDATE SET review_video_url=excluded.review_video_url`, id, *in.ReviewVideoURL, a.opts.Billing.SupportPlans())
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 0 {
					return problem{409, "Cette copie ne bénéficie pas du suivi vidéo."}
				}
			}
		}
		out, e = scanSubmission(tx.QueryRow(ctx, "SELECT "+submissionFields("$2")+" FROM submission_tracking WHERE id=$1", id, a.opts.Billing.SupportPlans()))
		return e
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, out)
}
func (a *App) serveFile(w http.ResponseWriter, r *http.Request, key, mime string) error {
	if a.store == nil {
		return unavailable()
	}
	body, _, err := a.store.Get(r.Context(), key)
	if err != nil {
		return unavailable()
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Security-Policy", "sandbox")
	_, err = io.Copy(w, body)
	if err != nil {
		a.logger.Warn("file transfer interrupted", "route", r.Pattern)
	}
	return nil
}
func (a *App) submissionImage(w http.ResponseWriter, r *http.Request, u User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var key, mime string
	err = a.db.QueryRow(r.Context(), "SELECT object_key,content_type FROM submissions WHERE id=$1 AND object_key<>'' AND (user_id=$2 OR $3='admin')", id, u.ID, u.Role).Scan(&key, &mime)
	if err != nil {
		return err
	}
	return a.serveFile(w, r, key, mime)
}
func (a *App) addDocument(w http.ResponseWriter, r *http.Request, _ User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	f, err := multipartFields(w, r, map[string]int{"file": pdfLimit, "title": 300}, pdfLimit+300)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(string(f["title"]))
	if title == "" || !utf8.ValidString(title) {
		return bad("Titre du document requis.")
	}
	data := f["file"]
	if !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.Contains(data[max(0, len(data)-1024):], []byte("%%EOF")) {
		return problem{415, "Document PDF attendu."}
	}
	ctx := r.Context()
	key, err := a.stage(ctx, "documents", "application/pdf", data)
	if err != nil {
		return err
	}
	var doc Document
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		if err := contentLock(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE content SET status='draft' WHERE id=$1 AND NOT archived", id); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO content_documents(content_id,title,size,object_key) SELECT id,$2,$3,$4 FROM content WHERE id=$1 AND NOT archived RETURNING id::text,title,size`, id, title, len(data), key).Scan(&doc.ID, &doc.Title, &doc.Size); err != nil {
			return err
		}
		return attach(ctx, tx, key)
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 201, doc)
}
func (a *App) document(w http.ResponseWriter, r *http.Request, u User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var key string
	err = a.db.QueryRow(r.Context(), `SELECT object_key FROM content_documents d WHERE d.id=$1 AND ($2='admin' OR EXISTS(SELECT FROM published_content c WHERE c.id=d.content_id))`, id, u.Role).Scan(&key)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Disposition", `attachment; filename="document.pdf"`)
	return a.serveFile(w, r, key, "application/pdf")
}
func (a *App) deleteDocument(w http.ResponseWriter, r *http.Request, _ User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		if err := contentLock(ctx, tx); err != nil {
			return err
		}
		var key, cid string
		if err := tx.QueryRow(ctx, "DELETE FROM content_documents WHERE id=$1 RETURNING object_key,content_id::text", id).Scan(&key, &cid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE content SET status='draft' WHERE id=$1", cid); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO jobs(kind,payload,deduplication_key) SELECT 'delete_object',jsonb_build_object('key',$1::text),'delete/'||$1 WHERE NOT EXISTS(SELECT FROM content_documents WHERE object_key=$1) ON CONFLICT DO NOTHING`, key)
		return err
	})
	if err != nil {
		return err
	}
	return empty(w, 204)
}

func (a *App) submissionDetail(w http.ResponseWriter, r *http.Request, u User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	s, err := scanSubmission(a.db.QueryRow(r.Context(), "SELECT "+submissionFields("$4")+" FROM submission_tracking WHERE id=$1 AND (user_id=$2 OR $3='admin')", id, u.ID, u.Role, a.opts.Billing.SupportPlans()))
	if err != nil {
		return err
	}
	return jsonOut(w, 200, s)
}
