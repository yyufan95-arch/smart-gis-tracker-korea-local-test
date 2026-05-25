package web

import (
	"fmt"
	"net/http"
	"pointsldp/web/controller"
)
/*
func WebStart(app controller.Application) {
	// 加载静态资源（CSS、JS、图片）
	fs := http.FileServer(http.Dir("web/static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// 登录相关路由
	http.HandleFunc("/", app.LoginView)         // 登录页视图
	http.HandleFunc("/login", app.Login)        // 登录提交

	// 首页
	http.HandleFunc("/index", app.Index)

	// 区块链积分功能
	http.HandleFunc("/createAccount", app.CreateAccountHandler)
	http.HandleFunc("/mint", app.MintHandler)
	http.HandleFunc("/transfer", app.TransferHandler)
	http.HandleFunc("/getBalance", app.GetBalanceHandler)
	http.HandleFunc("/upload", app.UploadHandler)
	http.HandleFunc("/rank", app.RankHandler)
	http.HandleFunc("/api/getBalance", app.GetBalanceAPIHandler)
	http.HandleFunc("/api/getBalanceWithRank", app.GetBalanceWithRankHandler)
	http.HandleFunc("/api/rank", app.RankApiHandler)
        http.HandleFunc("/api/history", app.GetHistoryAPIHandler)

	http.HandleFunc("/auth/nonce", app.AuthNonceHandler)
	http.HandleFunc("/auth/login", app.AuthLoginHandler)

	http.Handle("/api/register", app.JWT(app.RegisterDIDHandler)) // 可选：若希望注册也登录后才可用，保留 JWT。若要开放，改为 HandleFunc。


	// 业务接口加 JWT 校验
	http.Handle("/api/getBalanceWithRank", app.JWT(app.GetBalanceWithRankHandler))
	http.Handle("/api/rank",              app.JWT(app.RankApiHandler))
	http.Handle("/api/history",           app.JWT(app.GetHistoryAPIHandler))





	// 启动服务器
	fmt.Println("✅ 웹 서비스가 시작되었습니다. 접속 주소: http://localhost:9000")
	err := http.ListenAndServe(":9000", nil)
	if err != nil {
		fmt.Printf("❌ Web服务启动失败: %v\n", err)
	}
}
*/
func WebStart(app controller.Application) {
	fs := http.FileServer(http.Dir("web/static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))


	// 首页
	http.HandleFunc("/index", app.Index)

	// 无鉴权的“登录流程”接口（获取 nonce、使用 DID 登录）
	http.HandleFunc("/auth/nonce", app.AuthNonceHandler)
	http.HandleFunc("/auth/login", app.AuthLoginHandler)

	// 注册：前端本地生成 DID 后，通知后端在 Fabric 上创建账户
//	http.Handle("/api/register", app.JWT(app.RegisterDIDHandler)) // 可选：若希望注册也登录后才可用，保留 JWT。若要开放，改为 HandleFunc。
//	http.HandleFunc("/api/register", app.RegisterDIDHandler)
	http.HandleFunc("/api/register-did", app.RegisterDIDOnLedgerHandler)

	// 业务接口 —— 统一加 JWT
	http.Handle("/api/getBalance",         app.JWT(app.GetBalanceAPIHandler))
	http.Handle("/api/getBalanceWithRank", app.JWT(app.GetBalanceWithRankHandler))
	http.Handle("/api/rank",               app.JWT(app.RankApiHandler))
	http.Handle("/api/history",            app.JWT(app.GetHistoryAPIHandler))

	// 这些页面表单接口（如果前端全走 Vue + API，也可以去掉）
	http.Handle("/createAccount", app.JWT(app.CreateAccountHandler))
	http.Handle("/mint",          app.JWT(app.MintHandler))
//	http.Handle("/transfer",      app.JWT(app.TransferHandler))
	http.Handle("/upload",        app.JWT(app.UploadHandler))
	http.Handle("/rank",          app.JWT(app.RankHandler))


	// 业务接口 —— 统一加 JWT
	http.Handle("/api/profile",    app.JWT(app.UpsertProfileHandler)) // POST
	http.Handle("/api/profile/me", app.JWT(app.GetProfileMeHandler))  // GET

	http.Handle("/api/profile/names", app.JWT(app.ProfileNamesHandler))

	http.HandleFunc("/api/recovery/find-did/initiate", app.RecoveryFindDIDInitiate)
	http.HandleFunc("/api/recovery/find-did/verify",   app.RecoveryFindDIDVerify)

	// —— 恢复备份：登录态直接读/存 ——（需要 JWT）
	// 注意：这两个用 Handle（不是 HandleFunc），因为 app.JWT 返回的是 http.Handler
	http.Handle("/api/recovery/blob/store", app.JWT(app.RecoveryBlobStore))
	http.Handle("/api/recovery/blob",       app.JWT(app.RecoveryBlobGet))

	 // 兼容前端调用的 /get 路径（关键！）
        http.Handle("/api/recovery/blob/get",   app.JWT(app.RecoveryBlobGet))

	// —— 无登录态下载备份：验证码校验后返回密文 ——（无需登录）
	http.HandleFunc("/api/recovery/blob/request", app.RecoveryBlobRequest)
	http.HandleFunc("/api/recovery/blob/verify",  app.RecoveryBlobVerify)

	http.HandleFunc("/api/ticket/batch", app.TicketBatchHandler)
	http.HandleFunc("/api/ticket", app.TicketGetHandler)
	http.HandleFunc("/api/tickets", app.TicketListHandler)
	http.Handle("/api/my-tickets", app.JWT(app.MyTicketsHandler))
	http.Handle("/api/ticket/assign", app.JWT(app.TicketAssignHandler))
	http.Handle("/api/ticket/use", app.JWT(app.TicketUseHandler))

	// 后台页面
	http.HandleFunc("/ticketTransfer", app.TicketTransferView)
	http.HandleFunc("/ticket/transfer", app.TicketTransferSubmit)

	// 前端自助 API（JWT）
	http.Handle("/api/ticket/transfer", app.JWT(app.TicketTransferAPIHandler))

	http.HandleFunc("/gate", app.GatePageHandler)                 // 检票员网页
	http.HandleFunc("/api/gate/ticket/use", app.GateTicketUseAPI) // 无登录核销 API

	http.Handle("/api/ticket/claim", app.JWT(app.TicketClaimHandler))

	http.HandleFunc("/api/recovery/passblob/store-pop", app.PassBlobStorePOP)
	http.HandleFunc("/api/recovery/passblob/get",       app.PassBlobGetByDID)



	http.HandleFunc("/transfer", app.TransferHandler)
	fmt.Println("✅ Web 서비스 시작: http://localhost:9010")
	if err := http.ListenAndServe(":9010", nil); err != nil {
		fmt.Printf("❌ Web服务启动失败: %v\n", err)
	}
}

