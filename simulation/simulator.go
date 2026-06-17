package simulation

import (
	"context"
	"fmt"
	"log"
	"math"
	"mortise-tenon-api/models"
	"mortise-tenon-api/solver"
	"time"
)

const (
	Gravity         = 9.81
	PoissonsRatio   = 0.35
	ThicknessMM     = 20.0
	WidthMM         = 100.0
	HeightMM        = 80.0
	Resolution      = 12
	SafetyFactorSF  = 1.5
	DefaultTimeout  = 2 * time.Second
	SolverTolerance = 1e-6
	MaxIterations   = 500
)

type JointSimulator struct {
	wood    models.WoodMaterial
	joint   models.JointType
	timeout time.Duration
}

type SimulationWarning struct {
	Message   string
	Level     string
	Timestamp time.Time
}

func NewJointSimulator(wood models.WoodMaterial, joint models.JointType) *JointSimulator {
	return &JointSimulator{
		wood:    wood,
		joint:   joint,
		timeout: DefaultTimeout,
	}
}

func (js *JointSimulator) SetTimeout(timeout time.Duration) {
	js.timeout = timeout
}

func (js *JointSimulator) Simulate() (*models.SimulationResult, error) {
	return js.SimulateWithContext(context.Background())
}

func (js *JointSimulator) SimulateWithContext(ctx context.Context) (*models.SimulationResult, error) {
	EPa := js.wood.ElasticModulusGPa * 1e9

	mesh := solver.GenerateJointMesh(WidthMM, HeightMM, js.joint.OverlapMM, Resolution)
	K := solver.AssembleStiffnessMatrix(mesh, EPa, PoissonsRatio, ThicknessMM)

	simCtx, cancel := context.WithTimeout(ctx, js.timeout)
	defer cancel()

	done := make(chan struct{})
	var tensileMaxStress, torsionMaxStress float64
	var warnings []SimulationWarning
	var usedFallback bool

	go func() {
		defer close(done)

		var tErr, torErr error
		tensileMaxStress, tErr = js.simulateTensionWithPCG(simCtx, mesh, K, EPa)
		if tErr != nil {
			select {
			case <-simCtx.Done():
				warnings = append(warnings, SimulationWarning{
					Message:   fmt.Sprintf("拉伸模拟超时: %v", tErr),
					Level:     "WARN",
					Timestamp: time.Now(),
				})
			default:
				warnings = append(warnings, SimulationWarning{
					Message:   fmt.Sprintf("拉伸模拟求解失败: %v, 使用估算值", tErr),
					Level:     "WARN",
					Timestamp: time.Now(),
				})
			}
			tensileMaxStress = js.estimateTensileStress(EPa)
			usedFallback = true
		}

		torsionMaxStress, torErr = js.simulateTorsionWithPCG(simCtx, mesh, K, EPa)
		if torErr != nil {
			select {
			case <-simCtx.Done():
				warnings = append(warnings, SimulationWarning{
					Message:   fmt.Sprintf("扭转模拟超时: %v", torErr),
					Level:     "WARN",
					Timestamp: time.Now(),
				})
			default:
				warnings = append(warnings, SimulationWarning{
					Message:   fmt.Sprintf("扭转模拟求解失败: %v, 使用估算值", torErr),
					Level:     "WARN",
					Timestamp: time.Now(),
				})
			}
			torsionMaxStress = js.estimateTorsionStress(EPa)
			usedFallback = true
		}
	}()

	select {
	case <-done:
	case <-simCtx.Done():
		<-done
		warnings = append(warnings, SimulationWarning{
			Message:   fmt.Sprintf("整体模拟超时(>%v), 全部使用估算值", js.timeout),
			Level:     "WARN",
			Timestamp: time.Now(),
		})
		tensileMaxStress = js.estimateTensileStress(EPa)
		torsionMaxStress = js.estimateTorsionStress(EPa)
		usedFallback = true
	}

	for _, w := range warnings {
		log.Printf("[%s] %s: %s [%s] 木材=%s, 榫卯=%s",
			w.Timestamp.Format(time.RFC3339), w.Level, w.Message,
			time.Since(w.Timestamp).Round(time.Millisecond),
			js.wood.Name, js.joint.Name)
	}

	maxLoadKg, failureMode, safetyFactor := js.calculateFailureLoad(
		tensileMaxStress, torsionMaxStress,
	)

	if usedFallback {
		safetyFactor *= 0.85
		safetyFactor = math.Round(safetyFactor*100) / 100
		maxLoadKg = math.Round(maxLoadKg*100) / 100
	}

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
		IsEstimated:      usedFallback,
		CalculatedAt:     time.Now(),
	}, nil
}

func (js *JointSimulator) simulateTensionWithPCG(ctx context.Context, mesh *solver.FEMesh2D, K *solver.SparseMatrix, EPa float64) (float64, error) {
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

	cancel := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(cancel)
	}()

	displacements, _, err := solver.PreconditionedConjugateGradientWithCancel(
		KCopy, F, SolverTolerance, MaxIterations, cancel,
	)
	if err != nil {
		return js.estimateTensileStress(EPa), err
	}

	stresses := solver.CalculateStresses(mesh, displacements, EPa, PoissonsRatio)
	maxStress := solver.MaxVonMisesStress(stresses)

	if maxStress < 1e-10 || math.IsNaN(maxStress) || math.IsInf(maxStress, 0) {
		return js.estimateTensileStress(EPa), fmt.Errorf("invalid stress result: %v", maxStress)
	}

	tenonFactor := js.calculateTenonFactor()
	return maxStress * tenonFactor, nil
}

func (js *JointSimulator) simulateTorsionWithPCG(ctx context.Context, mesh *solver.FEMesh2D, K *solver.SparseMatrix, EPa float64) (float64, error) {
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

	cancel := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(cancel)
	}()

	displacements, _, err := solver.PreconditionedConjugateGradientWithCancel(
		KCopy, F, SolverTolerance, MaxIterations, cancel,
	)
	if err != nil {
		return js.estimateTorsionStress(EPa), err
	}

	stresses := solver.CalculateStresses(mesh, displacements, EPa, PoissonsRatio)
	maxStress := solver.MaxVonMisesStress(stresses)

	if maxStress < 1e-10 || math.IsNaN(maxStress) || math.IsInf(maxStress, 0) {
		return js.estimateTorsionStress(EPa), fmt.Errorf("invalid stress result: %v", maxStress)
	}

	torsionFactor := js.calculateTorsionFactor()
	return maxStress * torsionFactor, nil
}

func (js *JointSimulator) estimateTensileStress(EPa float64) float64 {
	area := WidthMM * ThicknessMM * js.joint.WidthRatio * js.joint.DepthRatio
	areaM2 := area * 1e-6
	testLoadN := 1000.0
	nominalStress := testLoadN / areaM2

	teethFactor := 1.0 + 0.03*float64(js.joint.TeethCount)
	depthFactor := 0.7 + 0.5*js.joint.DepthRatio
	stressConcentration := 1.5 + 0.5*(1.0-js.joint.WidthRatio)

	angleRad := js.joint.AngleDeg * math.Pi / 180.0
	angleFactor := 1.0 + 0.3*math.Sin(angleRad)

	estimated := nominalStress * teethFactor * depthFactor * stressConcentration * angleFactor
	return estimated
}

func (js *JointSimulator) estimateTorsionStress(EPa float64) float64 {
	widthM := WidthMM * 1e-3
	heightM := HeightMM * 1e-3
	testMomentN := 500.0
	sectionModulus := (widthM * heightM * heightM) / 6.0
	nominalStress := testMomentN / sectionModulus

	teethFactor := 1.0 + 0.02*float64(js.joint.TeethCount)
	depthFactor := 0.8 + 0.4*js.joint.DepthRatio
	stressConcentration := 1.8 + 0.4*(1.0-js.joint.WidthRatio)

	angleRad := js.joint.AngleDeg * math.Pi / 180.0
	angleFactor := 1.0 + 0.4*math.Cos(angleRad)
	frictionFactor := 0.85 + 0.3*js.wood.FrictionCoeff

	estimated := nominalStress * teethFactor * depthFactor * stressConcentration * angleFactor * frictionFactor
	return estimated
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

	slipResistance := js.calculateSlipResistance()

	woodTearLoad := (tensileStrength / tensileStressPa) * 1000.0 / Gravity
	tenonBreakLoad := (shearStrength / torsionStressPa) * 1500.0 / Gravity
	slipLoad := slipResistance / Gravity

	woodTearLoad *= js.joint.WidthRatio * js.joint.DepthRatio
	tenonBreakLoad *= (0.5 + 0.1*float64(js.joint.TeethCount))

	if math.IsNaN(woodTearLoad) || math.IsInf(woodTearLoad, 0) {
		woodTearLoad = js.estimateMaxLoad() * 0.6
	}
	if math.IsNaN(tenonBreakLoad) || math.IsInf(tenonBreakLoad, 0) {
		tenonBreakLoad = js.estimateMaxLoad() * 0.7
	}
	if math.IsNaN(slipLoad) || math.IsInf(slipLoad, 0) {
		slipLoad = js.estimateMaxLoad() * 0.8
	}

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

func (js *JointSimulator) estimateMaxLoad() float64 {
	area := WidthMM * ThicknessMM * js.joint.WidthRatio * js.joint.DepthRatio
	areaM2 := area * 1e-6
	maxForce := js.wood.TensileStrengthPa * areaM2 * 0.4
	return maxForce / Gravity
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
