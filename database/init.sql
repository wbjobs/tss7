CREATE DATABASE IF NOT EXISTS mortise_tenon;

\c mortise_tenon;

CREATE TABLE IF NOT EXISTS simulation_history (
    id BIGSERIAL PRIMARY KEY,
    wood_type VARCHAR(50) NOT NULL,
    joint_type VARCHAR(50) NOT NULL,
    max_load_kg DOUBLE PRECISION NOT NULL,
    failure_mode VARCHAR(20) NOT NULL,
    safety_factor DOUBLE PRECISION NOT NULL,
    tensile_stress_max_pa DOUBLE PRECISION NOT NULL,
    torsion_stress_max_pa DOUBLE PRECISION NOT NULL,
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
END $$;

CREATE INDEX IF NOT EXISTS idx_simulation_history_wood_type ON simulation_history(wood_type);
CREATE INDEX IF NOT EXISTS idx_simulation_history_joint_type ON simulation_history(joint_type);
CREATE INDEX IF NOT EXISTS idx_simulation_history_calculated_at ON simulation_history(calculated_at DESC);
CREATE INDEX IF NOT EXISTS idx_simulation_history_is_estimated ON simulation_history(is_estimated);
