package main

import (
	"fmt"
	"log"
	"mortise-tenon-api/database"
	"mortise-tenon-api/handlers"
	"mortise-tenon-api/models"
	"net/http"
	"os"
	"strings"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var db *database.Database
	var err error

	useDB := os.Getenv("DISABLE_DB") != "true"
	if useDB {
		db, err = database.NewDatabase()
		if err != nil {
			log.Printf("警告: 数据库连接失败，将以无数据库模式运行: %v", err)
			log.Println("如需禁用数据库，请设置环境变量 DISABLE_DB=true")
			db = nil
		} else {
			defer db.Close()
			log.Println("数据库连接成功")
		}
	}

	handler := handlers.NewAPIHandler(db)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if strings.HasPrefix(r.URL.Path, "/api/history/") {
				handler.GetHistoryByID(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintf(w, `{
  "name": "古董木工榫卯结构应力模拟API",
  "version": "1.1.0",
  "endpoints": {
    "POST /api/simulate": "执行应力模拟计算（含湿度影响分析）",
    "GET  /api/materials": "获取支持的木材种类列表",
    "GET  /api/joints": "获取支持的榫卯类型列表",
    "GET  /api/history": "获取计算历史记录",
    "GET  /api/history/{id}": "获取指定历史记录详情",
    "GET  /health": "健康检查"
  },
  "simulate_request": {
    "wood_type": "string (必填，如橡木、胡桃木)",
    "joint_type": "string (必填，如燕尾榫、直榫)",
    "humidity_rh": "float64 (可选，默认50，范围30-90)"
  },
  "humidity_features": [
    "湿度-木材膨胀系数映射",
    "动态过盈量（装配紧密度）计算",
    "预压应力边界条件",
    "建议涂蜡等级输出",
    "历史数据湿度字段统计"
  ],
  "supported_woods": %s,
  "supported_joints": %s,
  "default_humidity_rh": 50,
  "humidity_range": "30%% - 90%%"
}`,
			toJSON(models.ListWoodMaterials()),
			toJSON(models.ListJointTypes()),
		)
	})

	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/api/simulate", handler.Simulate)
	mux.HandleFunc("/api/materials", handler.ListMaterials)
	mux.HandleFunc("/api/joints", handler.ListJoints)
	mux.HandleFunc("/api/history", handler.GetHistory)

	log.Println("========================================")
	log.Println("  古董木工榫卯结构应力模拟API")
	log.Println("========================================")
	log.Printf("  服务端口: %s", port)
	log.Printf("  支持木材: %s", strings.Join(models.ListWoodMaterials(), ", "))
	log.Printf("  支持榫卯: %s", strings.Join(models.ListJointTypes(), ", "))
	if db != nil {
		log.Println("  数据库: 已连接")
	} else {
		log.Println("  数据库: 未启用")
	}
	log.Println("========================================")
	log.Printf("服务已启动，监听 http://localhost:%s", port)
	log.Println()

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

func toJSON(v interface{}) string {
	if s, ok := v.([]string); ok {
		return "[" + joinQuoted(s) + "]"
	}
	return "[]"
}

func joinQuoted(ss []string) string {
	for i := range ss {
		ss[i] = `"` + ss[i] + `"`
	}
	return strings.Join(ss, ", ")
}
