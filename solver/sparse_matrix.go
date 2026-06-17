package solver

import (
	"fmt"
	"math"
	"time"
)

type SparseMatrix struct {
	Rows    int
	Cols    int
	Values  []float64
	ColIdx  []int
	RowPtr  []int
}

func NewSparseMatrix(rows, cols int) *SparseMatrix {
	return &SparseMatrix{
		Rows:   rows,
		Cols:   cols,
		Values: make([]float64, 0),
		ColIdx: make([]int, 0),
		RowPtr: make([]int, rows+1),
	}
}

func (m *SparseMatrix) Set(row, col int, value float64) {
	if row < 0 || row >= m.Rows || col < 0 || col >= m.Cols {
		panic(fmt.Sprintf("index out of bounds: row=%d, col=%d", row, col))
	}

	start := m.RowPtr[row]
	end := m.RowPtr[row+1]

	for i := start; i < end; i++ {
		if m.ColIdx[i] == col {
			m.Values[i] = value
			return
		}
	}

	pos := start
	for pos < end && m.ColIdx[pos] < col {
		pos++
	}

	m.Values = append(m.Values, 0)
	m.ColIdx = append(m.ColIdx, 0)
	copy(m.Values[pos+1:], m.Values[pos:])
	copy(m.ColIdx[pos+1:], m.ColIdx[pos:])
	m.Values[pos] = value
	m.ColIdx[pos] = col

	for i := row + 1; i <= m.Rows; i++ {
		m.RowPtr[i]++
	}
}

func (m *SparseMatrix) Add(row, col int, value float64) {
	if row < 0 || row >= m.Rows || col < 0 || col >= m.Cols {
		panic(fmt.Sprintf("index out of bounds: row=%d, col=%d", row, col))
	}

	start := m.RowPtr[row]
	end := m.RowPtr[row+1]

	for i := start; i < end; i++ {
		if m.ColIdx[i] == col {
			m.Values[i] += value
			return
		}
	}

	pos := start
	for pos < end && m.ColIdx[pos] < col {
		pos++
	}

	m.Values = append(m.Values, 0)
	m.ColIdx = append(m.ColIdx, 0)
	copy(m.Values[pos+1:], m.Values[pos:])
	copy(m.ColIdx[pos+1:], m.ColIdx[pos:])
	m.Values[pos] = value
	m.ColIdx[pos] = col

	for i := row + 1; i <= m.Rows; i++ {
		m.RowPtr[i]++
	}
}

func (m *SparseMatrix) Get(row, col int) float64 {
	if row < 0 || row >= m.Rows || col < 0 || col >= m.Cols {
		panic(fmt.Sprintf("index out of bounds: row=%d, col=%d", row, col))
	}

	start := m.RowPtr[row]
	end := m.RowPtr[row+1]

	for i := start; i < end; i++ {
		if m.ColIdx[i] == col {
			return m.Values[i]
		}
	}

	return 0.0
}

func (m *SparseMatrix) MultiplyVector(v []float64) []float64 {
	if len(v) != m.Cols {
		panic(fmt.Sprintf("vector size mismatch: expected %d, got %d", m.Cols, len(v)))
	}

	result := make([]float64, m.Rows)
	for row := 0; row < m.Rows; row++ {
		start := m.RowPtr[row]
		end := m.RowPtr[row+1]
		sum := 0.0
		for i := start; i < end; i++ {
			sum += m.Values[i] * v[m.ColIdx[i]]
		}
		result[row] = sum
	}
	return result
}

func (m *SparseMatrix) ToDense() [][]float64 {
	dense := make([][]float64, m.Rows)
	for i := range dense {
		dense[i] = make([]float64, m.Cols)
	}
	for row := 0; row < m.Rows; row++ {
		start := m.RowPtr[row]
		end := m.RowPtr[row+1]
		for i := start; i < end; i++ {
			dense[row][m.ColIdx[i]] = m.Values[i]
		}
	}
	return dense
}

func (m *SparseMatrix) Transpose() *SparseMatrix {
	t := NewSparseMatrix(m.Cols, m.Rows)
	for row := 0; row < m.Rows; row++ {
		start := m.RowPtr[row]
		end := m.RowPtr[row+1]
		for i := start; i < end; i++ {
			t.Set(m.ColIdx[i], row, m.Values[i])
		}
	}
	return t
}

func (m *SparseMatrix) NNZ() int {
	return len(m.Values)
}

func (m *SparseMatrix) Diagonal() []float64 {
	diag := make([]float64, m.Rows)
	for row := 0; row < m.Rows; row++ {
		start := m.RowPtr[row]
		end := m.RowPtr[row+1]
		for i := start; i < end; i++ {
			if m.ColIdx[i] == row {
				diag[row] = m.Values[i]
				break
			}
		}
	}
	return diag
}

func (m *SparseMatrix) AddRegularization(epsilon float64) {
	avgDiag := 0.0
	diag := m.Diagonal()
	count := 0
	for _, d := range diag {
		if math.Abs(d) > 1e-15 {
			avgDiag += math.Abs(d)
			count++
		}
	}
	if count > 0 {
		avgDiag /= float64(count)
	}
	shift := avgDiag * epsilon
	if shift < 1e-10 {
		shift = epsilon
	}
	for row := 0; row < m.Rows; row++ {
		found := false
		start := m.RowPtr[row]
		end := m.RowPtr[row+1]
		for i := start; i < end; i++ {
			if m.ColIdx[i] == row {
				m.Values[i] += shift
				found = true
				break
			}
		}
		if !found {
			m.Set(row, row, shift)
		}
	}
}

func (m *SparseMatrix) ConditionNumberEstimate() float64 {
	diag := m.Diagonal()
	maxDiag := 0.0
	minDiag := math.Inf(1)
	for _, d := range diag {
		absD := math.Abs(d)
		if absD > maxDiag {
			maxDiag = absD
		}
		if absD > 1e-15 && absD < minDiag {
			minDiag = absD
		}
	}
	if minDiag == math.Inf(1) || minDiag < 1e-15 {
		return math.Inf(1)
	}
	return maxDiag / minDiag
}

func VectorDot(a, b []float64) float64 {
	if len(a) != len(b) {
		panic("vector sizes don't match for dot product")
	}
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func VectorNorm(v []float64) float64 {
	return math.Sqrt(VectorDot(v, v))
}

func VectorAdd(a, b []float64) []float64 {
	if len(a) != len(b) {
		panic("vector sizes don't match for addition")
	}
	result := make([]float64, len(a))
	for i := range a {
		result[i] = a[i] + b[i]
	}
	return result
}

func VectorSub(a, b []float64) []float64 {
	if len(a) != len(b) {
		panic("vector sizes don't match for subtraction")
	}
	result := make([]float64, len(a))
	for i := range a {
		result[i] = a[i] - b[i]
	}
	return result
}

func VectorScale(v []float64, s float64) []float64 {
	result := make([]float64, len(v))
	for i := range v {
		result[i] = v[i] * s
	}
	return result
}

func ConjugateGradient(A *SparseMatrix, b []float64, tol float64, maxIter int) ([]float64, int, error) {
	n := len(b)
	if A.Rows != n || A.Cols != n {
		return nil, 0, fmt.Errorf("matrix size mismatch")
	}

	x := make([]float64, n)
	r := VectorSub(b, A.MultiplyVector(x))
	p := make([]float64, n)
	copy(p, r)
	rsOld := VectorDot(r, r)

	if rsOld < tol*tol {
		return x, 0, nil
	}

	for iter := 0; iter < maxIter; iter++ {
		Ap := A.MultiplyVector(p)
		pAp := VectorDot(p, Ap)

		if math.Abs(pAp) < 1e-15 {
			return x, iter, fmt.Errorf("breakdown in CG: p'Ap ~ 0")
		}

		alpha := rsOld / pAp
		x = VectorAdd(x, VectorScale(p, alpha))
		r = VectorSub(r, VectorScale(Ap, alpha))

		rsNew := VectorDot(r, r)

		if math.Sqrt(rsNew) < tol {
			return x, iter + 1, nil
		}

		beta := rsNew / rsOld
		p = VectorAdd(r, VectorScale(p, beta))
		rsOld = rsNew
	}

	return x, maxIter, fmt.Errorf("CG did not converge in %d iterations", maxIter)
}

func JacobiSolver(A *SparseMatrix, b []float64, tol float64, maxIter int) ([]float64, int, error) {
	return JacobiSolverWithCancel(A, b, tol, maxIter, nil)
}

func JacobiSolverWithCancel(A *SparseMatrix, b []float64, tol float64, maxIter int, cancel <-chan struct{}) ([]float64, int, error) {
	n := len(b)
	if A.Rows != n || A.Cols != n {
		return nil, 0, fmt.Errorf("matrix size mismatch")
	}

	x := make([]float64, n)
	xNew := make([]float64, n)

	for iter := 0; iter < maxIter; iter++ {
		select {
		case <-cancel:
			return x, iter, fmt.Errorf("solver cancelled")
		default:
		}

		for i := 0; i < n; i++ {
			sum := 0.0
			aDiag := 0.0

			start := A.RowPtr[i]
			end := A.RowPtr[i+1]
			for j := start; j < end; j++ {
				col := A.ColIdx[j]
				val := A.Values[j]
				if col == i {
					aDiag = val
				} else {
					sum += val * x[col]
				}
			}

			if math.Abs(aDiag) < 1e-15 {
				return nil, iter, fmt.Errorf("zero diagonal element at row %d", i)
			}

			xNew[i] = (b[i] - sum) / aDiag
		}

		diff := 0.0
		for i := 0; i < n; i++ {
			diff += (xNew[i] - x[i]) * (xNew[i] - x[i])
			x[i] = xNew[i]
		}

		if math.Sqrt(diff) < tol {
			return x, iter + 1, nil
		}
	}

	return x, maxIter, fmt.Errorf("Jacobi did not converge in %d iterations", maxIter)
}

func PreconditionedConjugateGradient(A *SparseMatrix, b []float64, tol float64, maxIter int) ([]float64, int, error) {
	return PreconditionedConjugateGradientWithCancel(A, b, tol, maxIter, nil)
}

func PreconditionedConjugateGradientWithCancel(A *SparseMatrix, b []float64, tol float64, maxIter int, cancel <-chan struct{}) ([]float64, int, error) {
	n := len(b)
	if A.Rows != n || A.Cols != n {
		return nil, 0, fmt.Errorf("matrix size mismatch")
	}

	diag := A.Diagonal()
	M := make([]float64, n)
	for i := 0; i < n; i++ {
		if math.Abs(diag[i]) > 1e-15 {
			M[i] = 1.0 / diag[i]
		} else {
			M[i] = 1.0
		}
	}

	x := make([]float64, n)
	r := VectorSub(b, A.MultiplyVector(x))

	z := make([]float64, n)
	for i := 0; i < n; i++ {
		z[i] = r[i] * M[i]
	}

	p := make([]float64, n)
	copy(p, z)

	rsOld := VectorDot(r, z)

	if math.Sqrt(rsOld) < tol {
		return x, 0, nil
	}

	for iter := 0; iter < maxIter; iter++ {
		select {
		case <-cancel:
			return x, iter, fmt.Errorf("solver cancelled")
		default:
		}

		Ap := A.MultiplyVector(p)
		pAp := VectorDot(p, Ap)

		if math.Abs(pAp) < 1e-15 {
			return x, iter, fmt.Errorf("breakdown in PCG: p'Ap ~ 0")
		}

		alpha := rsOld / pAp
		x = VectorAdd(x, VectorScale(p, alpha))
		r = VectorSub(r, VectorScale(Ap, alpha))

		rsNewMag := VectorNorm(r)
		if rsNewMag < tol {
			return x, iter + 1, nil
		}

		for i := 0; i < n; i++ {
			z[i] = r[i] * M[i]
		}

		rsNew := VectorDot(r, z)
		beta := rsNew / rsOld
		p = VectorAdd(z, VectorScale(p, beta))
		rsOld = rsNew
	}

	return x, maxIter, fmt.Errorf("PCG did not converge in %d iterations", maxIter)
}

func SolveWithTimeout(A *SparseMatrix, b []float64, tol float64, maxIter int, timeout time.Duration) ([]float64, int, error) {
	cancel := make(chan struct{})
	done := make(chan struct{})

	var result []float64
	var iters int
	var err error

	go func() {
		defer close(done)
		condNum := A.ConditionNumberEstimate()
		if condNum > 1e10 || math.IsInf(condNum, 1) {
			regA := copySparseMatrixInternal(A)
			regA.AddRegularization(1e-8)
			result, iters, err = PreconditionedConjugateGradientWithCancel(regA, b, tol, maxIter, cancel)
		} else {
			result, iters, err = PreconditionedConjugateGradientWithCancel(A, b, tol, maxIter, cancel)
		}
	}()

	select {
	case <-done:
		return result, iters, err
	case <-time.After(timeout):
		close(cancel)
		<-done
		return nil, 0, fmt.Errorf("solver timeout after %v", timeout)
	}
}

func copySparseMatrixInternal(m *SparseMatrix) *SparseMatrix {
	cp := NewSparseMatrix(m.Rows, m.Cols)
	cp.Values = make([]float64, len(m.Values))
	cp.ColIdx = make([]int, len(m.ColIdx))
	cp.RowPtr = make([]int, len(m.RowPtr))
	copy(cp.Values, m.Values)
	copy(cp.ColIdx, m.ColIdx)
	copy(cp.RowPtr, m.RowPtr)
	return cp
}
