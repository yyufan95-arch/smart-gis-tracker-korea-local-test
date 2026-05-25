package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"pointsldp/sdkInit"
	"pointsldp/service"
	"pointsldp/web"
	"pointsldp/web/controller"
)

const (
	// === 积分链码 ===
	pointsCCName    = "pointsldpcc"
	pointsCCVersion = "1.0.0"
	// === 门票链码 ===
	ticketCCName    = "ticketcc"
	ticketCCVersion = "1.0.0"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	orgs := []*sdkInit.OrgInfo{
		{
			OrgAdminUser:  "Admin",
			OrgName:       "Org1",
			OrgMspId:      "Org1MSP",
			OrgUser:       "User1",
			OrgPeerNum:    1,
			OrgAnchorFile: os.Getenv("GOPATH") + "/src/pointsldp/fixtures/channel-artifacts/Org1MSPanchors.tx",
		},
	}

	// ========= 基础网络信息（用于创建通道）=========
	baseInfo := sdkInit.SdkEnvInfo{
		ChannelID:     "mychannel",
		ChannelConfig: os.Getenv("GOPATH") + "/src/pointsldp/fixtures/channel-artifacts/channel.tx",
		Orgs:          orgs,

		OrdererAdminUser: "Admin",
		OrdererOrgName:   "OrdererOrg",
		OrdererEndpoint:  "orderer.example.com",

		// 下面这三个字段仅在生命周期里用
		ChaincodeID:      pointsCCName,
		ChaincodePath:    os.Getenv("GOPATH") + "/src/pointsldp/chaincode/points/",
		ChaincodeVersion: pointsCCVersion,
	}

	// ========= 门票链码的 env =========
	ticketInfo := sdkInit.SdkEnvInfo{
		ChannelID:        baseInfo.ChannelID,
		ChannelConfig:    baseInfo.ChannelConfig,
		Orgs:             baseInfo.Orgs,
		OrdererAdminUser: baseInfo.OrdererAdminUser,
		OrdererOrgName:   baseInfo.OrdererOrgName,
		OrdererEndpoint:  baseInfo.OrdererEndpoint,

		ChaincodeID:      ticketCCName,
		ChaincodePath:    os.Getenv("GOPATH") + "/src/pointsldp/chaincode/tickets/",
		ChaincodeVersion: ticketCCVersion,
	}

	// 1) 初始化 SDK
	sdk, err := sdkInit.Setup("config.yaml", &baseInfo)
	if err != nil {
		fmt.Println("SDK setup error:", err)
		os.Exit(-1)
	}

	// 2) 创建并加入通道（幂等）
	if err := sdkInit.CreateAndJoinChannel(&baseInfo); err != nil {
		fmt.Println("Create and join channel error:", err)
		os.Exit(-1)
	}

	// 3) 部署“积分链码”（幂等：已提交会跳过）
	if err := sdkInit.CreateCCLifecycle(&baseInfo, 1, false, sdk); err != nil {
		fmt.Println("Create lifecycle (points) error:", err)
		os.Exit(-1)
	}

	// 4) 部署“门票链码”（幂等：已提交会跳过）
	if err := sdkInit.CreateCCLifecycle(&ticketInfo, 1, false, sdk); err != nil {
		fmt.Println("Create lifecycle (ticket) error:", err)
		os.Exit(-1)
	}

	// 5) 初始化“积分服务层”
	serviceSetup, err := service.InitService(baseInfo.ChaincodeID, baseInfo.ChannelID, baseInfo.Orgs[0], sdk)
	if err != nil {
		fmt.Println("Init points service error:", err)
		os.Exit(-1)
	}

	// 6) 初始化“门票服务层”（复用同一 channel client）
	ticketSvc := &service.TicketService{
		Client:      serviceSetup.Client,
		ChaincodeID: ticketInfo.ChaincodeID,
	}

	// 给链码一点启动缓冲（InitRequired=true 时，提交后背书容器初次冷启动）
	fmt.Println("等待链码 Init 完成...")
	time.Sleep(5 * time.Second)

	// ==== Indy 解析器（可选） ====
	resolverURL := getenv("RESOLVER_URL", "http://localhost:9000")
	registrarURL := getenv("REGISTRAR_URL", resolverURL)
	fmt.Println("🔎 Resolver URL =", resolverURL)
	fmt.Println("🧾 Registrar URL =", registrarURL)

	if err := pingResolver(resolverURL); err != nil {
		fmt.Println("❌ Indy resolver ping 失败：", err)
	} else {
		fmt.Println("✅ Indy resolver 连通正常")
	}

	// 7) 启动 Web
	poolDID := getenv("TICKET_POOL_DID", "")
	app := controller.Application{
		Setup:     serviceSetup,
		JWTSecret: []byte("replace-with-a-strong-secret"),
		Resolver:  resolverURL,
		Registrar: registrarURL,
		Ticket:    ticketSvc,
		PoolDID:   poolDID,
	}

	fmt.Println("🚀 即将启动 Web 服务（9010端口），Resolver =", app.Resolver)
	web.WebStart(app)
}

// 仅保留这个（可选）连通性检查
func pingResolver(resolver string) error {
	url := strings.TrimRight(resolver, "/") + "/ledger/domain"
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("ping resolver failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("ping resolver non-200: %d, body=%s", resp.StatusCode, string(b))
	}
	return nil
}

