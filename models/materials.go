package models

type WoodMaterial struct {
	Name              string
	ElasticModulusGPa float64
	FrictionCoeff     float64
	DensityKgM3       float64
	ShearModulusGPa   float64
	TensileStrengthPa float64
	CompressiveStrPa  float64
	ShearStrengthPa   float64
}

type JointType struct {
	Name        string
	TeethCount  int
	AngleDeg    float64
	DepthRatio  float64
	WidthRatio  float64
	OverlapMM   float64
}

var WoodMaterials = map[string]WoodMaterial{
	"橡木": {
		Name:              "橡木",
		ElasticModulusGPa: 12.5,
		FrictionCoeff:     0.45,
		DensityKgM3:       750,
		ShearModulusGPa:   4.8,
		TensileStrengthPa: 80e6,
		CompressiveStrPa:  60e6,
		ShearStrengthPa:   12e6,
	},
	"胡桃木": {
		Name:              "胡桃木",
		ElasticModulusGPa: 10.2,
		FrictionCoeff:     0.42,
		DensityKgM3:       680,
		ShearModulusGPa:   4.1,
		TensileStrengthPa: 68e6,
		CompressiveStrPa:  52e6,
		ShearStrengthPa:   10e6,
	},
	"樱桃木": {
		Name:              "樱桃木",
		ElasticModulusGPa: 9.8,
		FrictionCoeff:     0.44,
		DensityKgM3:       620,
		ShearModulusGPa:   3.9,
		TensileStrengthPa: 65e6,
		CompressiveStrPa:  50e6,
		ShearStrengthPa:   9.5e6,
	},
	"红木": {
		Name:              "红木",
		ElasticModulusGPa: 13.2,
		FrictionCoeff:     0.48,
		DensityKgM3:       850,
		ShearModulusGPa:   5.2,
		TensileStrengthPa: 90e6,
		CompressiveStrPa:  70e6,
		ShearStrengthPa:   14e6,
	},
	"松木": {
		Name:              "松木",
		ElasticModulusGPa: 8.5,
		FrictionCoeff:     0.38,
		DensityKgM3:       480,
		ShearModulusGPa:   3.2,
		TensileStrengthPa: 55e6,
		CompressiveStrPa:  40e6,
		ShearStrengthPa:   7.5e6,
	},
}

var JointTypes = map[string]JointType{
	"燕尾榫": {
		Name:       "燕尾榫",
		TeethCount: 6,
		AngleDeg:   14.0,
		DepthRatio: 0.6,
		WidthRatio: 0.75,
		OverlapMM:  20.0,
	},
	"直榫": {
		Name:       "直榫",
		TeethCount: 1,
		AngleDeg:   90.0,
		DepthRatio: 0.55,
		WidthRatio: 0.6,
		OverlapMM:  18.0,
	},
	"半隐燕尾榫": {
		Name:       "半隐燕尾榫",
		TeethCount: 5,
		AngleDeg:   12.0,
		DepthRatio: 0.5,
		WidthRatio: 0.7,
		OverlapMM:  16.0,
	},
	"指榫": {
		Name:       "指榫",
		TeethCount: 12,
		AngleDeg:   90.0,
		DepthRatio: 0.65,
		WidthRatio: 0.85,
		OverlapMM:  22.0,
	},
	"框榫": {
		Name:       "框榫",
		TeethCount: 1,
		AngleDeg:   90.0,
		DepthRatio: 0.45,
		WidthRatio: 0.5,
		OverlapMM:  15.0,
	},
}

func GetWoodMaterial(name string) (WoodMaterial, bool) {
	mat, ok := WoodMaterials[name]
	return mat, ok
}

func GetJointType(name string) (JointType, bool) {
	jt, ok := JointTypes[name]
	return jt, ok
}

func ListWoodMaterials() []string {
	names := make([]string, 0, len(WoodMaterials))
	for k := range WoodMaterials {
		names = append(names, k)
	}
	return names
}

func ListJointTypes() []string {
	names := make([]string, 0, len(JointTypes))
	for k := range JointTypes {
		names = append(names, k)
	}
	return names
}
