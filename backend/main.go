package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/livekit/protocol/auth"
)

// 配置项 统一管理
const (
	apiKey       = getenv("LIVEKIT_API_KEY", "livekit-dev-key")
	apiSecret    = getenv("LIVEKIT_API_SECRET", "livekit-dev-secret-32chars-strong")
	serverPort   = ":6060"
	tokenValid   = 4 * time.Hour
	readTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
	idleTimeout  = 60 * time.Second
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	// 初始化路由
	mux := http.NewServeMux()

	// 健康检查接口
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"livekit-backend"}`))
	})

	// Token生成接口 核心业务
	mux.HandleFunc("/api/token", func(w http.ResponseWriter, r *http.Request) {
		// 统一跨域处理
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		// 处理预检请求
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 只允许GET请求
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"error":"仅支持GET请求"}`))
			return
		}

		// 获取参数
		roomName := r.URL.Query().Get("room")
		identity := r.URL.Query().Get("identity")
		userName := r.URL.Query().Get("name")

		// 参数校验
		if roomName == "" || identity == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"房间号(room)和用户ID(identity)为必填参数"}`))
			log.Printf("[WARN] 缺少必填参数 room=%s, identity=%s", roomName, identity)
			return
		}

		// 用户名默认值
		if userName == "" {
			userName = identity
		}

		



		// 生成Token
		at := auth.NewAccessToken(apiKey, apiSecret)
        
		// 声明一个布尔变量，用于为下面需要指针的字段取地址
		t := true 

		grant := &auth.VideoGrant{
			RoomJoin:       true,       // ✅ 注意这里：RoomJoin 是普通 bool，改回直接用 true
			Room:           roomName,   // 字符串正常赋值
			CanPublish:     &t,         // ✅ 权限字段是 *bool，继续使用 &t
			CanSubscribe:   &t,         // ✅ 继续使用 &t
			CanPublishData: &t,         // ✅ 继续使用 &t
		}

		at.SetVideoGrant(grant).
			SetIdentity(identity).
			SetName(userName).
			SetValidFor(tokenValid)
			


		// 生成JWT
		token, err := at.ToJWT()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"Token生成失败"}`))
			log.Printf("[ERROR] Token生成失败: %v", err)
			return
		}

		// 返回结果
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"token":"` + token + `"}`))
		log.Printf("[INFO] Token生成成功 room=%s, identity=%s", roomName, identity)
	})

	// 初始化HTTP服务
	server := &http.Server{
		Addr:         serverPort,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// 优雅退出处理
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 启动服务
	go func() {
		log.Printf("✅ LiveKit 后端服务已启动，监听端口 %s", serverPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] 服务启动失败: %v", err)
		}
	}()

	// 等待退出信号
	<-quit
	log.Println("⚠️  收到退出信号，正在关闭服务...")

	// 关闭服务
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] 服务关闭失败: %v", err)
	}

	log.Println("✅ 服务已安全退出")
}