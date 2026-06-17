package simulation

import (
	"math"
	"mortise-tenon-api/models"
	"mortise-tenon-api/solver"
	"time"
)

const (
	Gravity        = 9.81
	PoissonsRatio  = 0.35
	ThicknessMM    = 20.0
	WidthMM        = 100.0
	HeightMM       = 80.0
	Resolution     = 12
	SafetyFactorSF = 1.5
)

type JointSimulator struct {
	wood  models.WoodMaterial
	joint models.JointType
}

func NewJointSimulator(wood models.WoodMaterial, joint models.JointType) *JointSimulator {
	return &JointSimulator{
		wood:  wood,
		joint: joint,
	}
}

func (js *JointSimulator) Simulate() (*models.SimulationResult, error) {
	EPa := js.wood.ElasticModulusGPa * 1e9

	mesh := solver.GenerateJointMesh(WidthMM, HeightMM, js.joint.OverlapMM, Resolution)
	K := solver.AssembleStiffnessMatrix(mesh, EPa, PoissonsRatio, ThicknessMM)

	tensileMaxStress := js.simulateTension(mesh, K, EPa)
	torsionMaxStress := js.simulateTorsion(mesh, K, EPa)

	maxLoadKg, failureMode, safetyFactor := js.calculateFailureLoad(
		tensileMaxStress, torsionMaxStress,
	)

	return &models.SimulationResult{
		WoodType:         js.wood.Name,
		JointType:        js.joint.Name,
		MaxLoadKg:        maxLoadKg,
		FailureMode:      failureMode,
		SafetyFactor:     safetyFactor,
		TensileStressMax: tensileMaxStress,
		TorsionStressMax: torsionMaxStress,
		Nodes:            len(mesh.Nodes),
		MatrixSize:       K.Rows,
		CalculatedAt:     time.Now(),
	}, nil
}

func (js *JointSimulator) simulateTension(mesh *solver.FEMesh2D, K *solver.SparseMatrix, EPa float64) float64 {
	numDOF := len(mesh.Nodes) * 2
	F := make([]float64, numDOF)

	var fixedDOFs []int
	for i, node := range mesh.Nodes {
		if node.X < 1e-6 {
			fixedDOFs = append(fixedDOFs, i*2, i*2+1)
		}
	}

	testLoadN := 1000.0
	loadPerNode := testLoadN / float64(Resolution)
	for i, node := range mesh.Nodes {
		if node.X > WidthMM-1e-6 {
			F[i*2] = loadPerNode
		}
	}

	KCopy := copySparseMatrix(K)
	solver.ApplyDirichletBC(KCopy, F, fixedDOFs)

	displacements, _, err := solver.ConjugateGradient(KCopy, F, 1e-8, 1000)
	if err != nil {
		displacements, _, _ = solver.JacobiSolver(KCopy, F, 1e-8, 5000)
	}

	stresses := solver.CalculateStresses(mesh, displacements, EPa, PoissonsRatio)
	maxStress := solver.MaxVonMisesStress(stresses)

	tenonFactor := js.calculateTenonFactor()
	return maxStress * tenonFactor
}

func (js *JointSimulator) simulateTorsion(mesh *solver.FEMesh2D, K *solver.SparseMatrix, EPa float64) float64 {
	numDOF := len(mesh.Nodes) * 2
	F := make([]float64, numDOF)

	var fixedDOFs []int
	for i, node := range mesh.Nodes {
		if node.X < 1e-6 {
			fixedDOFs = append(fixedDOFs, i*2, i*2+1)
		}
	}

	testMomentN := 500.0
	for i, node := range mesh.Nodes {
		if node.X > WidthMM-1e-6 {
			centerY := HeightMM / 2.0
			F[i*2+1] = testMomentN * (node.Y - centerY) / (HeightMM * HeightMM / 12.0)
		}
	}

	KCopy := copySparseMatrix(K)
	solver.ApplyDirichletBC(KCopy, F, fixedDOFs)

	displacements, _, err := solver.ConjugateGradient(KCopy, F, 1e-8, 1000)
	if err != nil {
		displacements, _, _ = solver.JacobiSolver(KCopy, F, 1e-8, 5000)
	}

	stresses := solver.CalculateStresses(mesh, displacements, EPa, PoissonsRatio)
	maxStress := solver.MaxVonMisesStress(stresses)

	torsionFactor := js.calculateTorsionFactor()
	return maxStress * torsionFactor
}

func (js *JointSimulator) calculateTenonFactor() float64 {
	teethFactor := 1.0 + 0.05*float64(js.joint.TeethCount)
	depthFactor := 0.6 + 0.8*js.joint.DepthRatio
	widthFactor := 0.5 + 0.7*js.joint.WidthRatio

	angleRad := js.joint.AngleDeg * math.Pi / 180.0
	angleFactor := 0.8 + 0.4*math.Sin(angleRad)

	return teethFactor * depthFactor * widthFactor * angleFactor
}

func (js *JointSimulator) calculateTorsionFactor() float64 {
	teethFactor := 1.0 + 0.03*float64(js.joint.TeethCount)
	depthFactor := 0.7 + 0.6*js.joint.DepthRatio

	angleRad := js.joint.AngleDeg * math.Pi / 180.0
	angleFactor := 0.7 + 0.5*math.Cos(angleRad)

	frictionFactor := 0.8 + 0.5*js.wood.FrictionCoeff

	return teethFactor * depthFactor * angleFactor * frictionFactor
}

func (js *JointSimulator) calculateFailureLoad(
	tensileStressPa, torsionStressPa float64,
) (float64, models.FailureMode, float64) {

	tensileStrength := js.wood.TensileStrengthPa
	shearStrength := js.wood.ShearStrengthPa
	compressiveStrength := js.wood.CompressiveStrPa

	slipResistance := js.calculateSlipResistance()

	woodTearLoad := (tensileStrength / tensileStressPa) * 1000.0 / Gravity
	tenonBreakLoad := (shearStrength / torsionStressPa) * 1500.0 / Gravity
	slipLoad := slipResistance / Gravity

	woodTearLoad *= js.joint.WidthRatio * js.joint.DepthRatio
	tenonBreakLoad *= (0.5 + 0.1*float64(js.joint.TeethCount))

	designLoad := math.Min(math.Min(woodTearLoad, tenonBreakLoad), slipLoad)
	safeLoad := designLoad / SafetyFactorSF

	var failureMode models.FailureMode
	minLoad := math.Min(math.Min(woodTearLoad, tenonBreakLoad), slipLoad)
	if minLoad == woodTearLoad {
		failureMode = models.WoodTear
	} else if minLoad == tenonBreakLoad {
		failureMode = models.TenonBreak
	} else {
		failureMode = models.Slippage
	}

	actualSF := designLoad / safeLoad

	safeLoad = math.Round(safeLoad*100) / 100
	actualSF = math.Round(actualSF*100) / 100

	return safeLoad, failureMode, actualSF
}

func (js *JointSimulator) calculateSlipResistance() float64 {
	mu := js.wood.FrictionCoeff
	contactArea := WidthMM * ThicknessMM * js.joint.WidthRatio * js.joint.DepthRatio
	contactAreaM2 := contactArea * 1e-6

	normalPressure := js.wood.CompressiveStrPa * 0.3
	normalForce := normalPressure * contactAreaM2

	frictionForce := mu * normalForce

	angleRad := js.joint.AngleDeg * math.Pi / 180.0
	angleLocking := 0.0
	if js.joint.AngleDeg < 45 {
		angleLocking = 1.0 / math.Tan(angleRad)
	}
	lockingForce := normalForce * angleLocking * 0.3

	teethEnhancement := 1.0 + 0.08*float64(js.joint.TeethCount)

	return (frictionForce + lockingForce) * teethEnhancement
}

func copySparseMatrix(m *solver.SparseMatrix) *solver.SparseMatrix {
	cp := solver.NewSparseMatrix(m.Rows, m.Cols)
	cp.Values = make([]float64, len(m.Values))
	cp.ColIdx = make([]int, len(m.ColIdx))
	cp.RowPtr = make([]int, len(m.RowPtr))
	copy(cp.Values, m.Values)
	copy(cp.ColIdx, m.ColIdx)
	copy(cp.RowPtr, m.RowPtr)
	return cp
}
