// Command demo creates three labelled, unpublishable chapters when explicitly enabled.
package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/pierremarsotlyon1/crpe/backend/internal/database"
	"log"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	if os.Getenv("DEMO_MODE") != "true" {
		return fmt.Errorf("set DEMO_MODE=true to create development fixtures")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, os.Getenv("DATABASE_URL"), 1)
	if err != nil {
		return err
	}
	defer pool.Close()
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for i, title := range []string{"Nombres et calculs", "Grandeurs et mesures", "Géométrie"} {
			id := fmt.Sprintf("c094d1b2-1363-4c6d-b567-%012d", i+1)
			_, err := tx.Exec(ctx, `INSERT INTO content(id,exam_id,kind,title,body,position,demo) VALUES($1,'crpe','chapter',$2,'Chapitre de démonstration pour découvrir l’atelier de préparation.', $3,true) ON CONFLICT(id) DO NOTHING`, id, "Démo · "+title, i+1)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
