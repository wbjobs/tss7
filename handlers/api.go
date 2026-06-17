package handlers

import (
	"encoding/json"
	"log"
	"mortise-tenon-api/database"
	"mortise-tenon-api/models"
	"mortise-tenon-api/simulation"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type APIHandler struct {
	db *database.Database
}

func NewAPIHandler(db *database.Database) *APIHandler {
	return &APIHandler{db: db}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, models.APIResponse{
		Success: false,
		Message: message,
	})
}

func (h *APIHandler) Simulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "只允许POST请求")
		return
	}

	startTime := time.Now()

	var req models.SimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求体格式: "+err.Error())
		return
	}

	req.WoodType = strings.TrimSpace(req.WoodType)
	req.JointType = strings.TrimSpace(req.JointType)

	if req.WoodType == "" || req.JointType == "" {
		writeError(w, http.StatusBadRequest, "木材种类和榫卯类型均为必填参数")
		return
	}

	wood, ok := models.GetWoodMaterial(req.WoodType)
	if !ok {
		writeError(w, http.StatusBadRequest, "不支持的木材种类: "+req.WoodType+"。可用类型: "+strings.Join(models.ListWoodMaterials(), ", "))
		return
	}

	joint, ok := models.GetJointType(req.JointType)
	if !ok {
		writeError(w, http.StatusBadRequest, "不支持的榫卯类型: "+req.JointType+"。可用类型: "+strings.Join(models.ListJointTypes(), ", "))
		return
	}

	sim := simulation.NewJointSimulator(wood, joint)
	sim.SetTimeout(2 * time.Second)

	ctx := r.Context()
	result, err := sim.SimulateWithContext(ctx)
	if err != nil {
		log.Printf("ERROR: 模拟失败 木材=%s 榫卯=%s 错误=%v 耗时=%s",
			req.WoodType, req.JointType, err, time.Since(startTime))
		writeError(w, http.StatusInternalServerError, "模拟计算失败: "+err.Error())
		return
	}

	elapsed := time.Since(startTime)
	var statusMsg string
	if elapsed > 1800*time.Millisecond {
		statusMsg = "模拟计算完成(接近超时阈值)"
	} else {
		statusMsg = "模拟计算完成"
	}

	if h.db != nil {
		id, err := h.db.SaveSimulation(result)
		if err == nil {
			result.ID = id
		} else {
			log.Printf("WARN: 保存数据库失败 木材=%s 榫卯=%s 错误=%v",
				req.WoodType, req.JointType, err)
		}
	}

	log.Printf("INFO: 模拟完成 木材=%s 榫卯=%s 最大承重=%.2fkg 失效模式=%s 安全系数=%.2f 耗时=%s",
		req.WoodType, req.JointType, result.MaxLoadKg,
		result.FailureMode, result.SafetyFactor, elapsed)

	writeJSON(w, http.StatusOK, models.APIResponse{
		Success: true,
		Message: statusMsg,
		Data:    result,
		Metadata: map[string]interface{}{
			"available_woods":     models.ListWoodMaterials(),
			"available_joints":    models.ListJointTypes(),
			"material_properties": wood,
			"joint_parameters":    joint,
			"performance": map[string]interface{}{
				"elapsed_ms": elapsed.Milliseconds(),
				"timeout_ms": 2000,
				"near_timeout": elapsed > 1800*time.Millisecond,
			},
		},
	})
}

func (h *APIHandler) ListMaterials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "只允许GET请求")
		return
	}

	materials := make([]map[string]interface{}, 0)
	for name, mat := range models.WoodMaterials {
		materials = append(materials, map[string]interface{}{
			"name":                name,
			"elastic_modulus_gpa": mat.ElasticModulusGPa,
			"friction_coeff":      mat.FrictionCoeff,
			"density_kg_m3":       mat.DensityKgM3,
			"shear_modulus_gpa":   mat.ShearModulusGPa,
			"tensile_strength_pa": mat.TensileStrengthPa,
			"compressive_str_pa":  mat.CompressiveStrPa,
			"shear_strength_pa":   mat.ShearStrengthPa,
		})
	}

	writeJSON(w, http.StatusOK, models.APIResponse{
		Success: true,
		Message: "木材种类列表",
		Metadata: map[string]interface{}{
			"materials": materials,
			"count":     len(materials),
		},
	})
}

func (h *APIHandler) ListJoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "只允许GET请求")
		return
	}

	joints := make([]map[string]interface{}, 0)
	for name, jt := range models.JointTypes {
		joints = append(joints, map[string]interface{}{
			"name":        name,
			"teeth_count": jt.TeethCount,
			"angle_deg":   jt.AngleDeg,
			"depth_ratio": jt.DepthRatio,
			"width_ratio": jt.WidthRatio,
			"overlap_mm":  jt.OverlapMM,
		})
	}

	writeJSON(w, http.StatusOK, models.APIResponse{
		Success: true,
		Message: "榫卯类型列表",
		Metadata: map[string]interface{}{
			"joints": joints,
			"count":  len(joints),
		},
	})
}

func (h *APIHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "只允许GET请求")
		return
	}

	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "数据库连接未初始化")
		return
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	records, err := h.db.GetHistory(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取历史记录失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models.APIResponse{
		Success: true,
		Message: "计算历史记录",
		Metadata: map[string]interface{}{
			"records": records,
			"count":   len(records),
		},
	})
}

func (h *APIHandler) GetHistoryByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "只允许GET请求")
		return
	}

	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "数据库连接未初始化")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 {
		writeError(w, http.StatusBadRequest, "缺少记录ID")
		return
	}

	id, err := strconv.ParseInt(pathParts[len(pathParts)-1], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的记录ID")
		return
	}

	record, err := h.db.GetHistoryByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询记录失败: "+err.Error())
		return
	}

	if record == nil {
		writeError(w, http.StatusNotFound, "记录不存在")
		return
	}

	writeJSON(w, http.StatusOK, models.APIResponse{
		Success: true,
		Message: "记录详情",
		Data: &models.SimulationResult{
			ID:               record.ID,
			WoodType:         record.WoodType,
			JointType:        record.JointType,
			MaxLoadKg:        record.MaxLoadKg,
			FailureMode:      record.FailureMode,
			SafetyFactor:     record.SafetyFactor,
			TensileStressMax: record.TensileStressMax,
			TorsionStressMax: record.TorsionStressMax,
			Nodes:            record.Nodes,
			MatrixSize:       record.MatrixSize,
			IsEstimated:      record.IsEstimated,
			CalculatedAt:     record.CalculatedAt,
		},
	})
}

func (h *APIHandler) Health(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":    "running",
		"database":  "disconnected",
		"materials": len(models.WoodMaterials),
		"joints":    len(models.JointTypes),
	}

	if h.db != nil {
		status["database"] = "connected"
	}

	writeJSON(w, http.StatusOK, models.APIResponse{
		Success:  true,
		Message:  "服务运行正常",
		Metadata: status,
	})
}
