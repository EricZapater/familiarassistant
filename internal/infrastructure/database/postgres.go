package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ericzapater/familiarassistant/internal/domain"
	_ "github.com/lib/pq"
)

// PostgresRepository implementa domain.CacheRepository i domain.MealPlanRepository utilitzant PostgreSQL.
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository crea i inicialitza la connexió a la base de dades PostgreSQL.
func NewPostgresRepository(dsn string) (*PostgresRepository, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("error obrint la connexió a postgres: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("error pinging postgres: %w", err)
	}

	repo := &PostgresRepository{db: db}
	if err := repo.initSchema(ctx); err != nil {
		return nil, fmt.Errorf("error auto-creant taules a postgres: %w", err)
	}

	return repo, nil
}

func (r *PostgresRepository) initSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS cache_respostes (
			clau       TEXT PRIMARY KEY,
			resposta   TEXT NOT NULL,
			expira_el  TIMESTAMPTZ NOT NULL
		);

		CREATE TABLE IF NOT EXISTS pauta_nutricional (
			id          SERIAL PRIMARY KEY,
			dia_setmana TEXT NOT NULL,
			apat        TEXT NOT NULL,
			menu        TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_cache_expira ON cache_respostes (expira_el);
		CREATE INDEX IF NOT EXISTS idx_pauta_dia ON pauta_nutricional (LOWER(dia_setmana));
	`
	_, err := r.db.ExecContext(ctx, schema)
	return err
}

// Close tanca la connexió a la base de dades.
func (r *PostgresRepository) Close() error {
	return r.db.Close()
}

// Get recupera una entrada de la cache si no ha expirat.
func (r *PostgresRepository) Get(ctx context.Context, key string) (*domain.CacheEntry, error) {
	query := `
		SELECT clau, resposta, expira_el 
		FROM cache_respostes 
		WHERE clau = $1 AND expira_el > NOW()
	`

	log.Printf("[PostgreSQL Query] Executing SELECT on cache_respostes | Key: %s", key)
	var entry domain.CacheEntry
	err := r.db.QueryRowContext(ctx, query, key).Scan(&entry.Key, &entry.Response, &entry.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[PostgreSQL Result] Cache MISS (no s'ha trobat cap registre no expirat per a key=%s)", key)
			return nil, nil // Cache miss sense error
		}
		return nil, fmt.Errorf("error al consultar la cache (key=%s): %w", key, err)
	}

	log.Printf("[PostgreSQL Result] Cache HIT | Key: %s | Expira: %s | Resposta: %s",
		entry.Key, entry.ExpiresAt.Format("15:04:05 02-01-2006"), truncateText(entry.Response, 80))
	return &entry, nil
}

// Set insereix o actualitza (UPSERT) una entrada a la cache amb la seva data d'expiració.
func (r *PostgresRepository) Set(ctx context.Context, entry domain.CacheEntry) error {
	query := `
		INSERT INTO cache_respostes (clau, resposta, expira_el)
		VALUES ($1, $2, $3)
		ON CONFLICT (clau) 
		DO UPDATE SET resposta = EXCLUDED.resposta, expira_el = EXCLUDED.expira_el
	`

	log.Printf("[PostgreSQL Query] Executing UPSERT on cache_respostes | Key: %s | ExpiresAt: %s", entry.Key, entry.ExpiresAt.Format(time.RFC3339))
	_, err := r.db.ExecContext(ctx, query, entry.Key, entry.Response, entry.ExpiresAt)
	if err != nil {
		return fmt.Errorf("error desant a la cache (key=%s): %w", entry.Key, err)
	}

	log.Printf("[PostgreSQL Result] Entrada desada correctament a cache_respostes | Key: %s", entry.Key)
	return nil
}

// Flush buida tota la memòria cau de PostgreSQL.
func (r *PostgresRepository) Flush(ctx context.Context) error {
	query := `DELETE FROM cache_respostes`
	log.Printf("[PostgreSQL Query] Executing DELETE FROM cache_respostes (Flush Cache)")
	res, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("error buidant la cache de postgres: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	log.Printf("[PostgreSQL Result] Cache buidada amb èxit (%d files eliminades)", rowsAffected)
	return nil
}

// GetByDayOfWeek recupera els àpats programats per a un dia determinat de la setmana.
func (r *PostgresRepository) GetByDayOfWeek(ctx context.Context, day string) ([]domain.MealPlan, error) {
	query := `
		SELECT id, dia_setmana, apat, menu
		FROM pauta_nutricional
		WHERE LOWER(dia_setmana) = LOWER($1)
		ORDER BY id ASC
	`

	log.Printf("[PostgreSQL Query] Executing SELECT on pauta_nutricional | Day: %s", day)
	rows, err := r.db.QueryContext(ctx, query, day)
	if err != nil {
		return nil, fmt.Errorf("error consultant la pauta per al dia %s: %w", day, err)
	}
	defer rows.Close()

	var plans []domain.MealPlan
	for rows.Next() {
		var plan domain.MealPlan
		if err := rows.Scan(&plan.ID, &plan.DayOfWeek, &plan.Meal, &plan.Menu); err != nil {
			return nil, fmt.Errorf("error llegint fila de la pauta nutricional: %w", err)
		}
		plans = append(plans, plan)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterant sobre files de la pauta: %w", err)
	}

	log.Printf("[PostgreSQL Result] Obtingudes %d files de pauta_nutricional per al dia '%s':", len(plans), day)
	for i, p := range plans {
		log.Printf("  🥗 Pauta [%d]: Dia=%s | Àpat=%s | Menú=%s", i+1, p.DayOfWeek, p.Meal, p.Menu)
	}

	return plans, nil
}

// GetAll recupera tota la pauta nutricional de la setmana.
func (r *PostgresRepository) GetAll(ctx context.Context) ([]domain.MealPlan, error) {
	query := `
		SELECT id, dia_setmana, apat, menu
		FROM pauta_nutricional
		ORDER BY id ASC
	`

	log.Printf("[PostgreSQL Query] Executing SELECT ALL on pauta_nutricional")
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error consultant tota la pauta: %w", err)
	}
	defer rows.Close()

	var plans []domain.MealPlan
	for rows.Next() {
		var plan domain.MealPlan
		if err := rows.Scan(&plan.ID, &plan.DayOfWeek, &plan.Meal, &plan.Menu); err != nil {
			return nil, fmt.Errorf("error llegint fila de la pauta nutricional: %w", err)
		}
		plans = append(plans, plan)
	}

	log.Printf("[PostgreSQL Result] Obtingudes %d files en total de pauta_nutricional", len(plans))
	for i, p := range plans {
		log.Printf("  🥗 Pauta [%d]: Dia=%s | Àpat=%s | Menú=%s", i+1, p.DayOfWeek, p.Meal, p.Menu)
	}

	return plans, nil
}

func truncateText(text string, maxLen int) string {
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
