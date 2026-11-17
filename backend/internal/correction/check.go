// Package correction grounds automatic feedback in teacher-approved references.
package correction

import (
	"strings"

	"github.com/pierremarsotlyon1/crpe/backend/internal/grading"
)

type Error struct {
	Location string `json:"location"`
	Message  string `json:"message"`
}
type Result struct {
	GeneratedByAI      bool     `json:"generatedByAI"`
	Source             string   `json:"source"`
	Verdict            string   `json:"verdict"`
	Summary            string   `json:"summary"`
	Errors             []Error  `json:"errors"`
	WritingFeedback    []string `json:"writingFeedback"`
	ReferenceSolution  string   `json:"referenceSolution"`
	SimilarExerciseID  string   `json:"similarExerciseId,omitempty"`
	NeedsTeacherReview bool     `json:"needsTeacherReview"`
}

func normalize(v string) string { return strings.TrimSpace(strings.ReplaceAll(v, "\u00a0", " ")) }

// Check accepts only a final numeric value or a literal choice. A free-form
// proof or an unreadable answer requires human review, never string grading.
func Check(format, reference, answer string, tolerance float64) string {
	if normalize(answer) == "" || normalize(reference) == "" {
		return "uncertain"
	}
	switch format {
	case "choice":
		if normalize(answer) == normalize(reference) {
			return "correct"
		}
		return "incorrect"
	case "numeric":
		correct, known := grading.Numeric(reference, answer, tolerance)
		if !known {
			return "uncertain"
		}
		if correct {
			return "correct"
		}
		return "incorrect"
	default:
		return "uncertain"
	}
}

func localResult(format, reference, answer, solution string, tolerance float64) Result {
	r := Result{Source: "local_deterministic", Verdict: Check(format, reference, answer, tolerance), Errors: []Error{}, WritingFeedback: []string{}, ReferenceSolution: solution}
	switch r.Verdict {
	case "correct":
		r.Summary = "Le résultat correspond à la référence validée. La rédaction n’a pas été évaluée."
	case "incorrect":
		r.Summary = "Le résultat diffère de la référence validée."
		r.Errors = append(r.Errors, Error{Location: "Résultat final", Message: "Comparez votre résultat avec la solution de référence."})
	default:
		r.Summary = "Cette réponse nécessite une relecture par l’enseignante."
		r.NeedsTeacherReview = true
	}
	return r
}
