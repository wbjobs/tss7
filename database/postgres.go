package database

import (
	"database/sql"
	"fmt"
	"mortise-tenon-api/models"
	"os"

	_ "github.com/lib/pq"
)

type Database struct {
	conn *sql.DB
}

func NewDatabase() (*Database, error) {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "postgres")
	dbname := getEnv("DB_NAME", "mortise_tenon")
	sslmode := getEnv("DB_SSLMODE", "disable")

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	database := &Database{conn: db}
	if err := database.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return database, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (db *Database) initSchema() error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS simulation_history (
		id BIGSERIAL PRIMARY KEY,
		wood_type VARCHAR(50) NOT NULL,
		joint_type VARCHAR(50) NOT NULL,
		humidity_rh DOUBLE PRECISION NOT NULL DEFAULT 50.0,
		max_load_kg DOUBLE PRECISION NOT NULL,
		failure_mode VARCHAR(20) NOT NULL,
		safety_factor DOUBLE PRECISION NOT NULL,
		tensile_stress_max_pa DOUBLE PRECISION NOT NULL,
		torsion_stress_max_pa DOUBLE PRECISION NOT NULL,
		swelling_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.0,
		interference_mm DOUBLE PRECISION NOT NULL DEFAULT 0.0,
		recommended_wax_level VARCHAR(10) NOT NULL DEFAULT '无',
		nodes INTEGER NOT NULL,
		matrix_size INTEGER NOT NULL,
		is_estimated BOOLEAN NOT NULL DEFAULT FALSE,
		calculated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);

	DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'simulation_history' AND column_name = 'is_estimated'
		) THEN
			ALTER TABLE simulation_history ADD COLUMN is_estimated BOOLEAN NOT NULL DEFAULT FALSE;
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'simulation_history' AND column_name = 'humidity_rh'
		) THEN
			ALTER TABLE simulation_history ADD COLUMN humidity_rh DOUBLE PRECISION NOT NULL DEFAULT 50.0;
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'simulation_history' AND column_name = 'swelling_ratio'
		) THEN
			ALTER TABLE simulation_history ADD COLUMN swelling_ratio DOUBLE PRECISION NOT NULL DEFAULT 0.0;
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'simulation_history' AND column_name = 'interference_mm'
		) THEN
			ALTER TABLE simulation_history ADD COLUMN interference_mm DOUBLE PRECISION NOT NULL DEFAULT 0.0;
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'simulation_history' AND column_name = 'recommended_wax_level'
		) THEN
			ALTER TABLE simulation_history ADD COLUMN recommended_wax_level VARCHAR(10) NOT NULL DEFAULT '无';
		END IF;
	END $$;

	CREATE INDEX IF NOT EXISTS idx_simulation_history_wood_type ON simulation_history(wood_type);
	CREATE INDEX IF NOT EXISTS idx_simulation_history_joint_type ON simulation_history(joint_type);
	CREATE INDEX IF NOT EXISTS idx_simulation_history_humidity_rh ON simulation_history(humidity_rh);
	CREATE INDEX IF NOT EXISTS idx_simulation_history_calculated_at ON simulation_history(calculated_at DESC);
	CREATE INDEX IF NOT EXISTS idx_simulation_history_is_estimated ON simulation_history(is_estimated);
	`

	_, err := db.conn.Exec(createTableSQL)
	return err
}

func (db *Database) SaveSimulation(result *models.SimulationResult) (int64, error) {
	insertSQL := `
	INSERT INTO simulation_history (
		wood_type, joint_type, humidity_rh, max_load_kg, failure_mode, safety_factor,
		tensile_stress_max_pa, torsion_stress_max_pa, swelling_ratio, interference_mm,
		recommended_wax_level, nodes, matrix_size, is_estimated, calculated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	RETURNING id
	`

	var id int64
	err := db.conn.QueryRow(
		insertSQL,
		result.WoodType,
		result.JointType,
		result.HumidityRH,
		result.MaxLoadKg,
		string(result.FailureMode),
		result.SafetyFactor,
		result.TensileStressMax,
		result.TorsionStressMax,
		result.SwellingRatio,
		result.InterferenceMM,
		string(result.RecommendedWaxLevel),
		result.Nodes,
		result.MatrixSize,
		result.IsEstimated,
		result.CalculatedAt,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to insert simulation record: %w", err)
	}

	return id, nil
}

func (db *Database) GetHistory(limit int) ([]models.HistoryRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	querySQL := `
	SELECT id, wood_type, joint_type, humidity_rh, max_load_kg, failure_mode, safety_factor,
		tensile_stress_max_pa, torsion_stress_max_pa, swelling_ratio, interference_mm,
		recommended_wax_level, nodes, matrix_size, is_estimated, calculated_at
	FROM simulation_history
	ORDER BY calculated_at DESC
	LIMIT $1
	`

	rows, err := db.conn.Query(querySQL, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	var records []models.HistoryRecord
	for rows.Next() {
		var r models.HistoryRecord
		var failureMode, waxLevel string
		err := rows.Scan(
			&r.ID,
			&r.WoodType,
			&r.JointType,
			&r.HumidityRH,
			&r.MaxLoadKg,
			&failureMode,
			&r.SafetyFactor,
			&r.TensileStressMax,
			&r.TorsionStressMax,
			&r.SwellingRatio,
			&r.InterferenceMM,
			&waxLevel,
			&r.Nodes,
			&r.MatrixSize,
			&r.IsEstimated,
			&r.CalculatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		r.FailureMode = models.FailureMode(failureMode)
		r.RecommendedWaxLevel = models.WaxLevel(waxLevel)
		records = append(records, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return records, nil
}

func (db *Database) GetHistoryByID(id int64) (*models.HistoryRecord, error) {
	querySQL := `
	SELECT id, wood_type, joint_type, humidity_rh, max_load_kg, failure_mode, safety_factor,
		tensile_stress_max_pa, torsion_stress_max_pa, swelling_ratio, interference_mm,
		recommended_wax_level, nodes, matrix_size, is_estimated, calculated_at
	FROM simulation_history
	WHERE id = $1
	`

	var r models.HistoryRecord
	var failureMode, waxLevel string
	err := db.conn.QueryRow(querySQL, id).Scan(
		&r.ID,
		&r.WoodType,
		&r.JointType,
		&r.HumidityRH,
		&r.MaxLoadKg,
		&failureMode,
		&r.SafetyFactor,
		&r.TensileStressMax,
		&r.TorsionStressMax,
		&r.SwellingRatio,
		&r.InterferenceMM,
		&waxLevel,
		&r.Nodes,
		&r.MatrixSize,
		&r.IsEstimated,
		&r.CalculatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query record: %w", err)
	}

	r.FailureMode = models.FailureMode(failureMode)
	r.RecommendedWaxLevel = models.WaxLevel(waxLevel)
	return &r, nil
}

func (db *Database) Close() error {
	return db.conn.Close()
}
