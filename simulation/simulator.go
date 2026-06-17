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
	DefaultHumidity = 50.0
	BaseInterferenceMM = 0.15
)

type JointSimulator struct {
	wood       models.WoodMaterial
	joint      models.JointType
	humidityRH float64
	timeout    time.Duration
}

type SimulationWarning struct {
	Message   string
	Level     string
	Timestamp time.Time
}

func NewJointSimulator(wood models.WoodMaterial, joint models.JointType) *JointSimulator {
	return &JointSimulator{
		wood:       wood,
		joint:      joint,
		humidityRH: DefaultHumidity,
		timeout:    DefaultTimeout,
	}
}

func (js *JointSimulator) SetTimeout(timeout time.Duration) {
	js.timeout = timeout
}

func (js *JointSimulator) SetHumidity(humidityRH float64) {
	js.humidityRH = models.ClampHumidity(humidityRH)
}

func (js *JointSimulator) Simulate() (*models.SimulationResult, error) {
	return js.SimulateWithContext(context.Background())
}

func (js *JointSimulator) SimulateWithContext(ctx context.Context) (*models.SimulationResult, error) {
	EPa := js.wood.ElasticModulusGPa * 1e9

	swellingRatio := models.CalculateSwellingRatio(js.wood, js.humidityRH)
	baseTenonWidth := WidthMM * js.joint.WidthRatio
	interferenceMM := BaseInterferenceMM + models.CalculateInterference(swellingRatio, baseTenonWidth)

	if interferenceMM < 0 {
		interferenceMM = 0
	}

	waxLevel := models.GetRecommendedWaxLevel(js.humidityRH, swellingRatio, js.wood)

	tenonYCenterRatio := 0.5
	mesh := solver.GenerateJointMeshWithInterference(
		WidthMM, HeightMM, js.joint.OverlapMM, interferenceMM,
		js.joint.WidthRatio, tenonYCenterRatio, Resolution,
	)
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
		tensileMaxStress, tErr = js.simulateTensionWithPCG(
			simCtx, mesh, K, EPa, interferenceMM, tenonYCenterRatio,
		)
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
			tensileMaxStress = js.estimateTensileStress(EPa, interferenceMM)
			usedFallback = true
		}

		torsionMaxStress, torErr = js.simulateTorsionWithPCG(
			simCtx, mesh, K, EPa, interferenceMM, tenonYCenterRatio,
		)
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
			torsionMaxStress = js.estimateTorsionStress(EPa, interferenceMM)
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
		tensileMaxStress = js.estimateTensileStress(EPa, interferenceMM)
		torsionMaxStress = js.estimateTorsionStress(EPa, interferenceMM)
		usedFallback = true
	}

	for _, w := range warnings {
		log.Printf("[%s] %s: %s [%s] 木材=%s, 榫卯=%s, 湿度=%.1f%%",
			w.Timestamp.Format(time.RFC3339), w.Level, w.Message,
			time.Since(w.Timestamp).Round(time.Millisecond),
			js.wood.Name, js.joint.Name, js.humidityRH)
	}

	maxLoadKg, failureMode, safetyFactor := js.calculateFailureLoad(
		tensileMaxStress, torsionMaxStress, interferenceMM,
	)

	if usedFallback {
		safetyFactor *= 0.85
		safetyFactor = math.Round(safetyFactor*100) / 100
		maxLoadKg = math.Round(maxLoadKg*100) / 100
	}

	swellingRatio = math.Round(swellingRatio*10000) / 10000
	interferenceMM = math.Round(interferenceMM*1000) / 1000

	return &models.SimulationResult{
		WoodType:           js.wood.Name,
		JointType:          js.joint.Name,
		HumidityRH:         js.humidityRH,
		MaxLoadKg:          maxLoadKg,
		FailureMode:        failureMode,
		SafetyFactor:       safetyFactor,
		TensileStressMax:   tensileMaxStress,
		TorsionStressMax:   torsionMaxStress,
		SwellingRatio:      swellingRatio,
		InterferenceMM:     interferenceMM,
		RecommendedWaxLevel: waxLevel,
		Nodes:              len(mesh.Nodes),
		MatrixSize:         K.Rows,
		IsEstimated:        usedFallback,
		CalculatedAt:       time.Now(),
	}, nil
}

func (js *JointSimulator) simulateTensionWithPCG(
	ctx context.Context, mesh *solver.FEMesh2D, K *solver.SparseMatrix,
	EPa, interferenceMM, tenonYCenterRatio float64,
) (float64, error) {
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

	solver.ApplyInterferencePressure(
		mesh, F, WidthMM, HeightMM, js.joint.OverlapMM, interferenceMM,
		js.joint.WidthRatio, tenonYCenterRatio, EPa, PoissonsRatio,
	)

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
		return js.estimateTensileStress(EPa, interferenceMM), err
	}

	stresses := solver.CalculateStresses(mesh, displacements, EPa, PoissonsRatio)
	maxStress := solver.MaxVonMisesStress(stresses)

	if maxStress < 1e-10 || math.IsNaN(maxStress) || math.IsInf(maxStress, 0) {
		return js.estimateTensileStress(EPa, interferenceMM), fmt.Errorf("invalid stress result: %v", maxStress)
	}

	tenonFactor := js.calculateTenonFactor(interferenceMM)
	return maxStress * tenonFactor, nil
}

func (js *JointSimulator) simulateTorsionWithPCG(
	ctx context.Context, mesh *solver.FEMesh2D, K *solver.SparseMatrix,
	EPa, interferenceMM, tenonYCenterRatio float64,
) (float64, error) {
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

	solver.ApplyInterferencePressure(
		mesh, F, WidthMM, HeightMM, js.joint.OverlapMM, interferenceMM,
		js.joint.WidthRatio, tenonYCenterRatio, EPa, PoissonsRatio,
	)

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
		return js.estimateTorsionStress(EPa, interferenceMM), err
	}

	stresses := solver.CalculateStresses(mesh, displacements, EPa, PoissonsRatio)
	maxStress := solver.MaxVonMisesStress(stresses)

	if maxStress < 1e-10 || math.IsNaN(maxStress) || math.IsInf(maxStress, 0) {
		return js.estimateTorsionStress(EPa, interferenceMM), fmt.Errorf("invalid stress result: %v", maxStress)
	}

	torsionFactor := js.calculateTorsionFactor(interferenceMM)
	return maxStress * torsionFactor, nil
}

func (js *JointSimulator) estimateTensileStress(EPa, interferenceMM float64) float64 {
	area := WidthMM * ThicknessMM * js.joint.WidthRatio * js.joint.DepthRatio
	areaM2 := area * 1e-6
	testLoadN := 1000.0
	nominalStress := testLoadN / areaM2

	teethFactor := 1.0 + 0.03*float64(js.joint.TeethCount)
	depthFactor := 0.7 + 0.5*js.joint.DepthRatio
	stressConcentration := 1.5 + 0.5*(1.0-js.joint.WidthRatio)

	angleRad := js.joint.AngleDeg * math.Pi / 180.0
	angleFactor := 1.0 + 0.3*math.Sin(angleRad)

	interferenceFactor := 1.0 + 2.0*math.Abs(interferenceMM)/BaseInterferenceMM

	estimated := nominalStress * teethFactor * depthFactor * stressConcentration * angleFactor * interferenceFactor
	return estimated
}

func (js *JointSimulator) estimateTorsionStress(EPa, interferenceMM float64) float64 {
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

	interferenceFactor := 1.0 + 1.5*math.Abs(interferenceMM)/BaseInterferenceMM

	estimated := nominalStress * teethFactor * depthFactor * stressConcentration * angleFactor * frictionFactor * interferenceFactor
	return estimated
}

func (js *JointSimulator) calculateTenonFactor(interferenceMM float64) float64 {
	teethFactor := 1.0 + 0.05*float64(js.joint.TeethCount)
	depthFactor := 0.6 + 0.8*js.joint.DepthRatio
	widthFactor := 0.5 + 0.7*js.joint.WidthRatio

	angleRad := js.joint.AngleDeg * math.Pi / 180.0
	angleFactor := 0.8 + 0.4*math.Sin(angleRad)

	interferenceFactor := 1.0 + 1.5*math.Abs(interferenceMM)/BaseInterferenceMM

	return teethFactor * depthFactor * widthFactor * angleFactor * interferenceFactor
}

func (js *JointSimulator) calculateTorsionFactor(interferenceMM float64) float64 {
	teethFactor := 1.0 + 0.03*float64(js.joint.TeethCount)
	depthFactor := 0.7 + 0.6*js.joint.DepthRatio

	angleRad := js.joint.AngleDeg * math.Pi / 180.0
	angleFactor := 0.7 + 0.5*math.Cos(angleRad)

	frictionFactor := 0.8 + 0.5*js.wood.FrictionCoeff

	interferenceFactor := 1.0 + 1.2*math.Abs(interferenceMM)/BaseInterferenceMM

	return teethFactor * depthFactor * angleFactor * frictionFactor * interferenceFactor
}

func (js *JointSimulator) calculateFailureLoad(
	tensileStressPa, torsionStressPa, interferenceMM float64,
) (float64, models.FailureMode, float64) {

	tensileStrength := js.wood.TensileStrengthPa
	shearStrength := js.wood.ShearStrengthPa

	slipResistance := js.calculateSlipResistance(interferenceMM)

	woodTearLoad := (tensileStrength / tensileStressPa) * 1000.0 / Gravity
	tenonBreakLoad := (shearStrength / torsionStressPa) * 1500.0 / Gravity
	slipLoad := slipResistance / Gravity

	woodTearLoad *= js.joint.WidthRatio * js.joint.DepthRatio
	tenonBreakLoad *= (0.5 + 0.1*float64(js.joint.TeethCount))

	humidityFactor := 1.0
	rhDiff := math.Abs(js.humidityRH - js.wood.HumidityEffect.EquilibriumRH)
	if rhDiff > 20 {
		humidityFactor = 1.0 - (rhDiff-20.0)*0.005
		if humidityFactor < 0.6 {
			humidityFactor = 0.6
		}
	}

	woodTearLoad *= humidityFactor
	tenonBreakLoad *= humidityFactor

	if math.IsNaN(woodTearLoad) || math.IsInf(woodTearLoad, 0) {
		woodTearLoad = js.estimateMaxLoad(interferenceMM) * 0.6
	}
	if math.IsNaN(tenonBreakLoad) || math.IsInf(tenonBreakLoad, 0) {
		tenonBreakLoad = js.estimateMaxLoad(interferenceMM) * 0.7
	}
	if math.IsNaN(slipLoad) || math.IsInf(slipLoad, 0) {
		slipLoad = js.estimateMaxLoad(interferenceMM) * 0.8
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

func (js *JointSimulator) estimateMaxLoad(interferenceMM float64) float64 {
	area := WidthMM * ThicknessMM * js.joint.WidthRatio * js.joint.DepthRatio
	areaM2 := area * 1e-6
	maxForce := js.wood.TensileStrengthPa * areaM2 * 0.4
	interferenceFactor := 1.0 + 0.8*math.Abs(interferenceMM)/BaseInterferenceMM
	return maxForce * interferenceFactor / Gravity
}

func (js *JointSimulator) calculateSlipResistance(interferenceMM float64) float64 {
	mu := js.wood.FrictionCoeff
	contactArea := WidthMM * ThicknessMM * js.joint.WidthRatio * js.joint.DepthRatio
	contactAreaM2 := contactArea * 1e-6

	normalPressure := js.wood.CompressiveStrPa * 0.3

	interferencePressure := 0.0
	if interferenceMM > 1e-10 {
		EPa := js.wood.ElasticModulusGPa * 1e9
		interferencePressure = EPa * (interferenceMM / HeightMM) / (1 - PoissonsRatio*PoissonsRatio)
	}

	totalNormalPressure := normalPressure + interferencePressure
	normalForce := totalNormalPressure * contactAreaM2

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
