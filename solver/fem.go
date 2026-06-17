package solver

import (
	"math"
)

type Node2D struct {
	X, Y float64
}

type Element2D struct {
	NodeIDs [4]int
}

type FEMesh2D struct {
	Nodes    []Node2D
	Elements []Element2D
}

func GenerateJointMesh(widthMM, heightMM, overlapMM float64, resolution int) *FEMesh2D {
	mesh := &FEMesh2D{}

	nxX := resolution
	nxY := resolution

	dx := widthMM / float64(nxX-1)
	dy := heightMM / float64(nxY-1)

	for j := 0; j < nxY; j++ {
		for i := 0; i < nxX; i++ {
			mesh.Nodes = append(mesh.Nodes, Node2D{
				X: float64(i) * dx,
				Y: float64(j) * dy,
			})
		}
	}

	for j := 0; j < nxY-1; j++ {
		for i := 0; i < nxX-1; i++ {
			n0 := j*nxX + i
			n1 := j*nxX + i + 1
			n2 := (j+1)*nxX + i + 1
			n3 := (j+1)*nxX + i
			mesh.Elements = append(mesh.Elements, Element2D{
				NodeIDs: [4]int{n0, n1, n2, n3},
			})
		}
	}

	return mesh
}

func planeStressStiffnessMatrix(E, nu, t, a, b float64) *[8][8]float64 {
	var k [8][8]float64

	D := E / (1 - nu*nu)
	coeff := D * t / (4 * a * b)

	g1 := 1.0 / 3.0
	g2 := 1.0 / 6.0
	g3 := 1.0 / 4.0

	kxx := (b*b/a)*g1 + (1-nu)/2*(a*a/b)*g1
	kyy := (a*a/b)*g1 + (1-nu)/2*(b*b/a)*g1
	kxy := nu*g3 + (1-nu)/2*g3
	kyx := kxy

	a1 := (b*b/a)*g2 - (1-nu)/2*(a*a/b)*g1
	a2 := -(b*b/a)*g1 - (1-nu)/2*(a*a/b)*g2
	a3 := -(b*b/a)*g2 + (1-nu)/2*(a*a/b)*g2

	b1 := (a*a/b)*g2 - (1-nu)/2*(b*b/a)*g1
	b2 := -(a*a/b)*g1 - (1-nu)/2*(b*b/a)*g2
	b3 := -(a*a/b)*g2 + (1-nu)/2*(b*b/a)*g2

	c1 := nu*g3 - (1-nu)/2*g3
	c2 := -nu*g3 + (1-nu)/2*g3

	k[0][0] = kxx
	k[0][1] = kxy
	k[0][2] = a1
	k[0][3] = c2
	k[0][4] = a3
	k[0][5] = -kxy
	k[0][6] = a2
	k[0][7] = c1

	k[1][0] = kyx
	k[1][1] = kyy
	k[1][2] = c2
	k[1][3] = b1
	k[1][4] = -kyx
	k[1][5] = b3
	k[1][6] = c1
	k[1][7] = b2

	for i := 0; i < 8; i++ {
		for j := i + 1; j < 8; j++ {
			k[j][i] = k[i][j]
		}
	}

	k[2][2] = kxx
	k[2][3] = -kxy
	k[2][4] = a2
	k[2][5] = c2
	k[2][6] = a3
	k[2][7] = -c1

	k[3][3] = kyy
	k[3][4] = -c1
	k[3][5] = b2
	k[3][6] = -c2
	k[3][7] = b3

	k[4][4] = kxx
	k[4][5] = kxy
	k[4][6] = a1
	k[4][7] = c2

	k[5][5] = kyy
	k[5][6] = c2
	k[5][7] = b1

	k[6][6] = kxx
	k[6][7] = -kxy

	k[7][7] = kyy

	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			k[i][j] *= coeff
		}
	}

	return &k
}

func AssembleStiffnessMatrix(mesh *FEMesh2D, EPa, nu, thicknessMM float64) *SparseMatrix {
	numDOF := len(mesh.Nodes) * 2
	K := NewSparseMatrix(numDOF, numDOF)

	for _, elem := range mesh.Elements {
		x0 := mesh.Nodes[elem.NodeIDs[0]].X
		x1 := mesh.Nodes[elem.NodeIDs[1]].X
		y0 := mesh.Nodes[elem.NodeIDs[0]].Y
		y3 := mesh.Nodes[elem.NodeIDs[3]].Y

		a := (x1 - x0) / 2.0
		b := (y3 - y0) / 2.0

		ke := planeStressStiffnessMatrix(EPa, nu, thicknessMM, a, b)

		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				dofIx := elem.NodeIDs[i] * 2
				dofIy := elem.NodeIDs[i]*2 + 1
				dofJx := elem.NodeIDs[j] * 2
				dofJy := elem.NodeIDs[j]*2 + 1

				K.Add(dofIx, dofJx, ke[i*2][j*2])
				K.Add(dofIx, dofJy, ke[i*2][j*2+1])
				K.Add(dofIy, dofJx, ke[i*2+1][j*2])
				K.Add(dofIy, dofJy, ke[i*2+1][j*2+1])
			}
		}
	}

	return K
}

func ApplyDirichletBC(K *SparseMatrix, F []float64, fixedDOFs []int) {
	for _, dof := range fixedDOFs {
		for col := 0; col < K.Cols; col++ {
			K.Set(dof, col, 0.0)
		}
		for row := 0; row < K.Rows; row++ {
			K.Set(row, dof, 0.0)
		}
		K.Set(dof, dof, 1.0)
		F[dof] = 0.0
	}
}

func CalculateStresses(mesh *FEMesh2D, displacements []float64, EPa, nu float64) []float64 {
	numElems := len(mesh.Elements)
	stresses := make([]float64, numElems*3)

	for eIdx, elem := range mesh.Elements {
		u := make([]float64, 8)
		for i := 0; i < 4; i++ {
			u[i*2] = displacements[elem.NodeIDs[i]*2]
			u[i*2+1] = displacements[elem.NodeIDs[i]*2+1]
		}

		D := EPa / (1 - nu*nu)
		strainXX := (u[2] - u[0] + u[4] - u[6]) / 4.0
		strainYY := (u[5] - u[1] + u[7] - u[3]) / 4.0
		strainXY := (u[1] - u[0] + u[3] - u[2] + u[4] - u[5] + u[6] - u[7]) / 4.0

		stresses[eIdx*3] = D * (strainXX + nu*strainYY)
		stresses[eIdx*3+1] = D * (nu*strainXX + strainYY)
		stresses[eIdx*3+2] = D * (1 - nu) / 2.0 * strainXY
	}

	return stresses
}

func MaxVonMisesStress(stresses []float64) float64 {
	maxStress := 0.0
	for i := 0; i < len(stresses); i += 3 {
		sx := stresses[i]
		sy := stresses[i+1]
		txy := stresses[i+2]
		vonMises := math.Sqrt(sx*sx - sx*sy + sy*sy + 3*txy*txy)
		if vonMises > maxStress {
			maxStress = vonMises
		}
	}
	return maxStress
}
