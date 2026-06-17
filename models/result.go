package models

import "time"

type FailureMode string

const (
	WoodTear      FailureMode = "木材撕裂"
	TenonBreak    FailureMode = "榫头断裂"
	Slippage      FailureMode = "滑脱"
)

type SimulationResult struct {
	ID               int64       `json:"id,omitempty"`
	WoodType         string      `json:"wood_type"`
	JointType        string      `json:"joint_type"`
	MaxLoadKg        float64     `json:"max_load_kg"`
	FailureMode      FailureMode `json:"failure_mode"`
	SafetyFactor     float64     `json:"safety_factor"`
	TensileStressMax float64     `json:"tensile_stress_max_pa"`
	TorsionStressMax float64     `json:"torsion_stress_max_pa"`
	Nodes            int         `json:"nodes"`
	MatrixSize       int         `json:"matrix_size"`
	IsEstimated      bool        `json:"is_estimated"`
	CalculatedAt     time.Time   `json:"calculated_at"`
}

type SimulationRequest struct {
	WoodType  string `json:"wood_type"`
	JointType string `json:"joint_type"`
}

type APIResponse struct {
	Success  bool              `json:"success"`
	Message  string            `json:"message,omitempty"`
	Data     *SimulationResult `json:"data,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type HistoryRecord struct {
	ID               int64       `json:"id"`
	WoodType         string      `json:"wood_type"`
	JointType        string      `json:"joint_type"`
	MaxLoadKg        float64     `json:"max_load_kg"`
	FailureMode      FailureMode `json:"failure_mode"`
	SafetyFactor     float64     `json:"safety_factor"`
	TensileStressMax float64     `json:"tensile_stress_max_pa"`
	TorsionStressMax float64     `json:"torsion_stress_max_pa"`
	Nodes            int         `json:"nodes"`
	MatrixSize       int         `json:"matrix_size"`
	IsEstimated      bool        `json:"is_estimated"`
	CalculatedAt     time.Time   `json:"calculated_at"`
}
