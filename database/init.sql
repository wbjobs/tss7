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
    calculated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_simulation_history_wood_type ON simulation_history(wood_type);
CREATE INDEX IF NOT EXISTS idx_simulation_history_joint_type ON simulation_history(joint_type);
CREATE INDEX IF NOT EXISTS idx_simulation_history_calculated_at ON simulation_history(calculated_at DESC);
