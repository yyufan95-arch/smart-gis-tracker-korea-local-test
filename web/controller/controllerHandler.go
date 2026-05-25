package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"fmt"
	"sort"
	
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io/ioutil"
	"log"
	"math/big"
	"strings"
	"time"
	"sync"
	"github.com/btcsuite/btcutil/base58"
	"github.com/golang-jwt/jwt/v4"
	
	"pointsldp/service"

)
var cuser User

// 在你的 Application 结构体上补充三个字段
type Application struct {
    Setup       *service.ServiceSetup               // 你已有的 Fabric 封装
    JWTSecret   []byte               // 用于签发 JWT 的密钥
    Resolver    string               // 解析 DID→verkey 的服务地址，如 http://localhost:8080
    Registrar   string
    nonces      sync.Map             // 存放一次性挑战：key=did, value=nonceItem
    profiles    sync.Map   // key=did, value=Profile
    otps          sync.Map // key=字符串(邮箱或"did|email")=> otpItem
    recoveryBlobs sync.Map // key=did => RecoveryBlob

    Ticket *service.TicketService // ← 新增：门票服务
    PoolDID string

    passBlobs    sync.Map // key=did => RecoveryBlob（沿用你的结构）

}

type otpItem struct {
    Code    string
    Exp     time.Time
    DID     string // 可选：和 DID 绑定的找回使用
    Channel string // "email"|"phone"
    Contact string // 邮箱或手机号
}

type RecoveryBlob struct {
    DID       string    `json:"did"`
    CipherB64 string    `json:"cipherB64"`
    SaltB64   string    `json:"saltB64"`
    IVB64     string    `json:"ivB64"`
    UpdatedAt time.Time `json:"updatedAt"`
}


type nonceItem struct {
    Nonce string
    Exp   time.Time
}


type RankedAccount struct {
        ID      string
        Balance uint64
}

type Profile struct {
    DID         string    `json:"did"`
    DisplayName string    `json:"displayName,omitempty"`
    Email       string    `json:"email,omitempty"`
    Phone       string    `json:"phone,omitempty"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}

func (app *Application) LoginView(w http.ResponseWriter, r *http.Request)  {

	ShowView(w, r, "login.html", nil)
}

func (app *Application) Index(w http.ResponseWriter, r *http.Request)  {
	ShowView(w, r, "index.html", nil)
}

// 显示门票转账页面（后台高权限）
func (app *Application) TicketTransferView(w http.ResponseWriter, r *http.Request) {
    ShowView(w, r, "ticketTransfer.html", nil)
}

func (app *Application) Login(w http.ResponseWriter, r *http.Request) {
	loginName := r.FormValue("loginName")
	password := r.FormValue("password")

	var flag bool
	for _, user := range users {
		if user.LoginName == loginName && user.Password == password {
			cuser = user
			flag = true
			break
		}
	}

	data := &struct {
		CurrentUser User
		Flag bool
	}{
		CurrentUser:cuser,
		Flag:false,
	}

	if flag {
		// 登录成功
		ShowView(w, r, "index.html", data)
	}else{
		// 登录失败
		data.Flag = true
		data.CurrentUser.LoginName = loginName
		ShowView(w, r, "login.html", data)
	}
}

// 创建账户
func (app *Application) CreateAccountHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	userID := r.FormValue("userID")

	txID, err := app.Setup.CreateAccount(userID)

	data := struct {
		TxID string
		Msg  string
		Flag bool
	}{
		TxID: txID,
		Flag: err == nil,
	}

	if err != nil {
		data.Msg = "创建账户失败: " + err.Error()
	} else {
		data.Msg = "账户创建成功，交易ID: " + txID
	}

	ShowView(w, r, "createAccount.html", data)
}

// 铸造积分
func (app *Application) MintHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	toID := r.FormValue("toID")
	amountStr := r.FormValue("amount")
	amount, _ := strconv.ParseUint(amountStr, 10, 64)

	txID, err := app.Setup.Mint(toID, amount)

	data := struct {
		TxID string
		Msg  string
		Flag bool
	}{
		TxID: txID,
		Flag: err == nil,
	}

	if err != nil {
		data.Msg = "铸币失败: " + err.Error()
	} else {
		data.Msg = "铸币成功，交易ID: " + txID
	}

	ShowView(w, r, "mint.html", data)
}

// 积分转账
func (app *Application) TransferHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	fromID := r.FormValue("fromID")
	toID := r.FormValue("toID")
	amountStr := r.FormValue("amount")
	amount, _ := strconv.ParseUint(amountStr, 10, 64)

	txID, err := app.Setup.Transfer(fromID, toID, amount)

	data := struct {
		TxID string
		Msg  string
		Flag bool
	}{
		TxID: txID,
		Flag: err == nil,
	}

	if err != nil {
		data.Msg = "转账失败: " + err.Error()
	} else {
		data.Msg = "转账成功，交易ID: " + txID
	}

	ShowView(w, r, "transfer.html", data)
}
func (app *Application) GetBalanceAPIHandler(w http.ResponseWriter, r *http.Request) {
    // 允许跨域请求，开发阶段可用，生产环境需严格设置域名
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Content-Type", "application/json")

    // 获取 userID 参数
    userID := r.URL.Query().Get("userID")
    if userID == "" {
        http.Error(w, `{"error": "userID 不能为空"}`, http.StatusBadRequest)
        return
    }

    // 查询余额
    balanceBytes, err := app.Setup.GetBalance(userID)
    if err != nil {
        http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
        return
    }

    // 解析 JSON
    var result struct {
        ID      string `json:"id"`
        Balance int    `json:"balance"`
        DocType string `json:"docType"`
    }

    err = json.Unmarshal(balanceBytes, &result)
    if err != nil {
        http.Error(w, fmt.Sprintf(`{"error": "结果解析失败: %s"}`, err.Error()), http.StatusInternalServerError)
        return
    }

    // 返回 JSON
    json.NewEncoder(w).Encode(map[string]interface{}{
        "userID":  result.ID,
        "balance": result.Balance,
    })
}

func (app *Application) GetBalanceHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	userID := r.FormValue("userID")

	balanceBytes, err := app.Setup.GetBalance(userID)

	data := struct {
		UserID  string
		Balance string
		Msg     string
		Flag    bool
	}{
		UserID: userID,
		Flag:   err == nil,
	}

	if err != nil {
		data.Msg = "此账户不存在"
	} else {
		// 创建一个接收结构体
		var result struct {
			ID      string `json:"id"`
			Balance int    `json:"balance"`
			DocType string `json:"docType"`
		}

		// 将 JSON 字节反序列化为结构体
		err := json.Unmarshal(balanceBytes, &result)
		if err != nil {
			data.Msg = "查询结果解析失败: " + err.Error()
		} else {
			data.Balance = strconv.Itoa(result.Balance)
			data.Msg = "查询成功"
		}
	}

	ShowView(w, r, "getBalance.html", data)
}
// 上传文件并奖励积分
func (app *Application) UploadHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 限制最大10MB

	userID := r.FormValue("userID")

	fmt.Println("接收到 userID:", userID)

	file, fileHeader, err := r.FormFile("file")

	if fileHeader != nil {
		fmt.Println("接收到文件名:", fileHeader.Filename)
	}




	defer func() {
		if file != nil {
			file.Close()
		}
	}()

	data := struct {
		Msg string
		Flag bool
	}{
		Flag: false,
	}

	if err != nil {
		data.Msg = "文件上传失败: " + err.Error()
		ShowView(w, r, "upload.html", data)
		return
	}

	// ✅ 模拟识别通过，系统决定奖励 10 分
	txID, err := app.Setup.Transfer("user001", userID, 10)
	if err != nil {
		data.Msg = "转账失败: " + err.Error()
	} else {
		data.Msg = "上传成功，已奖励10积分！交易ID: " + txID
		data.Flag = true
	}

	ShowView(w, r, "upload.html", data)
}


func (app *Application) RankHandler(w http.ResponseWriter, r *http.Request) {
	accountsBytes, err := app.Setup.GetAllAccounts()

	data := struct {
		Accounts []RankedAccount
		Msg      string
		Flag     bool
	}{
		Flag: false,
	}

	if err != nil {
		data.Msg = "获取账户列表失败: " + err.Error()
		ShowView(w, r, "rank.html", data)
		return
	}

	var rawAccounts []struct {
		ID      string `json:"id"`
		Balance uint64 `json:"balance"`
	}

	if err := json.Unmarshal(accountsBytes, &rawAccounts); err != nil {
		data.Msg = "解析账户列表失败: " + err.Error()
		ShowView(w, r, "rank.html", data)
		return
	}

	// 排序
	sort.Slice(rawAccounts, func(i, j int) bool {
		return rawAccounts[i].Balance > rawAccounts[j].Balance
	})

	// 构造展示数据
	for _, acc := range rawAccounts {
		data.Accounts = append(data.Accounts, RankedAccount{
			ID: acc.ID, Balance: acc.Balance,
		})
	}
	data.Flag = true
	data.Msg = "排行榜加载成功"

	ShowView(w, r, "rank.html", data)
}
// GetBalanceWithRankHandler 返回某用户的积分与排名
func (app *Application) GetBalanceWithRankHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userID")
	if userID == "" {
		http.Error(w, "missing userID parameter", http.StatusBadRequest)
		return
	}

	accountsBytes, err := app.Setup.GetAllAccounts()
	if err != nil {
		http.Error(w, "获取账户列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var rawAccounts []struct {
		ID      string `json:"id"`
		Balance uint64 `json:"balance"`
	}

	if err := json.Unmarshal(accountsBytes, &rawAccounts); err != nil {
		http.Error(w, "解析账户列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 排序：按积分从高到低
	sort.Slice(rawAccounts, func(i, j int) bool {
		return rawAccounts[i].Balance > rawAccounts[j].Balance
	})

	var rank int = -1
	var balance uint64 = 0

	// 遍历找目标账户
	for i, acc := range rawAccounts {
		if acc.ID == userID {
			rank = i + 1 // 排名从1开始
			balance = acc.Balance
			break
		}
	}

	if rank == -1 {
		http.Error(w, "未找到该用户ID", http.StatusNotFound)
		return
	}

	// 返回 JSON 格式
	result := map[string]interface{}{
		"userID":  userID,
		"balance": balance,
		"rank":    rank,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (app *Application) RankApiHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "只支持 GET 方法", http.StatusMethodNotAllowed)
        return
    }

    accountsBytes, err := app.Setup.GetAllAccounts()
    if err != nil {
        http.Error(w, "获取账户列表失败: "+err.Error(), http.StatusInternalServerError)
        return
    }

    var rawAccounts []struct {
        ID      string `json:"id"`
        Balance uint64 `json:"balance"`
    }

    if err := json.Unmarshal(accountsBytes, &rawAccounts); err != nil {
        http.Error(w, "解析账户列表失败: "+err.Error(), http.StatusInternalServerError)
        return
    }

    sort.Slice(rawAccounts, func(i, j int) bool {
        return rawAccounts[i].Balance > rawAccounts[j].Balance
    })

    maxCount :=10
    if len(rawAccounts) > maxCount {
	    rawAccounts = rawAccounts[:maxCount]
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(rawAccounts)
}
func (app *Application) GetHistoryAPIHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Content-Type", "application/json")

    userID := r.URL.Query().Get("userID")
    if userID == "" {
        http.Error(w, `{"error": "userID 不能为空"}`, http.StatusBadRequest)
        return
    }

    historyBytes, err := app.Setup.GetHistory(userID)
    if err != nil {
        http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
        return
    }

    w.Write(historyBytes)
}

func (app *Application) AuthNonceHandler(w http.ResponseWriter, r *http.Request) {
    type req  struct{ DID string `json:"did"` }
    type resp struct{ Nonce string `json:"nonce"`; TS int64 `json:"ts"` }

    if r.Method != http.MethodPost {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusMethodNotAllowed)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
        return
    }

    var body req
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.DID) == "" {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
        return
    }
    log.Printf("[/auth/nonce] DID=%s", body.DID)

    n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
    nonce := n.Text(16)
    app.nonces.Store(body.DID, nonceItem{Nonce: nonce, Exp: time.Now().Add(60 * time.Second)})

    ts := time.Now().UnixNano() / int64(time.Millisecond)

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(resp{Nonce: nonce, TS: ts})
}

func (app *Application) AuthLoginHandler(w http.ResponseWriter, r *http.Request) {
    type req struct {
        DID   string `json:"did"`
        Nonce string `json:"nonce"`
        TS    int64  `json:"ts"`
        Sig   string `json:"sig"`
    }
    type resp struct {
        Token string `json:"token"`
        DID   string `json:"did"`
    }

    if r.Method != http.MethodPost {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusMethodNotAllowed)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
        return
    }

    var body req
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
        return
    }
    log.Printf("[/auth/login] DID=%s", body.DID)

    v, ok := app.nonces.Load(body.DID)
    if !ok {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "nonce not found"})
        return
    }
    item := v.(nonceItem)
    if item.Nonce != body.Nonce || time.Now().After(item.Exp) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "nonce expired/invalid"})
        return
    }
    app.nonces.Delete(body.DID)

    verkey, err := app.resolveVerkey(body.DID)
    if err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "resolve verkey failed"})
        return
    }

    msg := []byte(body.DID + "|" + body.Nonce + "|" + strconv.FormatInt(body.TS, 10))
    sigBytes, err := base64.StdEncoding.DecodeString(body.Sig)
    if err != nil || len(sigBytes) != ed25519.SignatureSize {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "bad signature"})
        return
    }
    if !ed25519.Verify(verkey, msg, sigBytes) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "verify failed"})
        return
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "sub": body.DID,
        "iat": time.Now().Unix(),
        "exp": time.Now().Add(2 * time.Hour).Unix(),
    })
    ss, err := token.SignedString(app.JWTSecret)
    if err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "token sign failed"})
        return
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(resp{Token: ss, DID: body.DID})
}


// resolveVerkey 直连 von-browser 的 /ledger/domain，解析 results 列表
func (app *Application) resolveVerkey(did string) (ed25519.PublicKey, error) {
    if app.Resolver == "" {
        return nil, errors.New("Resolver not set")
    }
    // 你这边 /ledger/domain ?did=... 是工作的
    url := strings.TrimRight(app.Resolver, "/") + "/ledger/domain?did=" + did

    resp, err := http.Get(url)
    if err != nil {
        return nil, fmt.Errorf("resolver http get failed: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := ioutil.ReadAll(resp.Body)
        log.Printf("resolver non-200: %d %s", resp.StatusCode, string(b))
        return nil, fmt.Errorf("resolver status=%d", resp.StatusCode)
    }

    // 结构按你贴的 JSON 来定义
    var out struct {
        Results []struct {
            Txn struct {
                Data struct {
                    Dest   string `json:"dest"`
                    Verkey string `json:"verkey"`
                } `json:"data"`
            } `json:"txn"`
        } `json:"results"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, fmt.Errorf("decode resolver json failed: %w", err)
    }

    // 在 results 里找 dest==did 的那条
    var verkeyB58 string
    for _, r := range out.Results {
        if r.Txn.Data.Dest == did {
            verkeyB58 = r.Txn.Data.Verkey
            break
        }
    }
    if verkeyB58 == "" {
        return nil, fmt.Errorf("resolver: DID %s not found in results", did)
    }

    // 处理缩写 verkey（以 ~ 开头时需要拼回完整 verkey）
    if strings.HasPrefix(verkeyB58, "~") {
        // Indy 约定：fullVerkey = b58( b58(DID) || b58(verkeyTail) )
        didBytes := base58.Decode(did)
        tailBytes := base58.Decode(verkeyB58[1:])
        full := append(didBytes, tailBytes...)
        verkeyB58 = base58.Encode(full)
    }

    pub := base58.Decode(verkeyB58)
    if len(pub) != ed25519.PublicKeySize {
        return nil, fmt.Errorf("bad verkey size: got %d, want %d", len(pub), ed25519.PublicKeySize)
    }
    return ed25519.PublicKey(pub), nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}


// 用法： http.Handle("/api/rank", app.JWT(app.RankApiHandler))
func (app *Application) JWT(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		raw := strings.TrimPrefix(auth, "Bearer ")

		tok, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
			// ✅ 更稳妥的算法校验
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return app.JWTSecret, nil
		})
		if err != nil || !tok.Valid {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
/*
// POST /api/register  -> body: { did, alias? }
func (app *Application) RegisterDIDHandler(w http.ResponseWriter, r *http.Request) {
	type req struct {
		DID   string `json:"did"`
		Alias string `json:"alias"`
	}
	type resp struct {
		OK   bool   `json:"ok"`
		DID  string `json:"did"`
		Msg  string `json:"msg"`
		TxID string `json:"txId,omitempty"`
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DID == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// 这里可以调用 von-network 的 /register 把 DID 写入 ledger（若还没写过）
	// 最小化起步：假设 DID 已在 von-network UI 注册成功；直接在 Fabric 初始化账户
	txID, err := app.Setup.CreateAccount(body.DID)
	if err != nil {
		writeJSON(w, resp{OK: false, DID: body.DID, Msg: "fabric 创建账户失败: " + err.Error()})
		return
	}
	writeJSON(w, resp{OK: true, DID: body.DID, Msg: "注册成功", TxID: txID})
}
*/


// POST /api/register-did
func (app *Application) RegisterDIDOnLedgerHandler(w http.ResponseWriter, r *http.Request) {
    type req struct {
        DID    string `json:"did"`
        Verkey string `json:"verkey"`        // base58 (32B)
        Alias  string `json:"alias,omitempty"`
        Nonce  string `json:"nonce"`
        TS     int64  `json:"ts"`
        Sig    string `json:"sig"`           // base64(ed25519)
    }
    type resp struct {
        OK   bool   `json:"ok"`
        DID  string `json:"did"`
        Msg  string `json:"msg"`
        TxID string `json:"txId,omitempty"`
    }

    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    var body req
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeJSON(w, resp{OK: false, DID: body.DID, Msg: "invalid request"})
        return
    }
    did := strings.TrimSpace(body.DID)
    verkeyB58 := strings.TrimSpace(body.Verkey)
    if did == "" || verkeyB58 == "" || body.Nonce == "" || body.Sig == "" || body.TS == 0 {
        writeJSON(w, resp{OK: false, DID: did, Msg: "missing fields"})
        return
    }

    // 1) 校验一次性挑战（防重放）
    v, ok := app.nonces.Load(did)
    if !ok {
        writeJSON(w, resp{OK: false, DID: did, Msg: "nonce not found"})
        return
    }
    item := v.(nonceItem)
    if item.Nonce != body.Nonce || time.Now().After(item.Exp) {
        writeJSON(w, resp{OK: false, DID: did, Msg: "nonce expired/invalid"})
        return
    }
    app.nonces.Delete(did)

    // 2) PoP：用 verkey 验证 did|nonce|ts 的签名
    pub := base58.Decode(verkeyB58)
    if len(pub) != ed25519.PublicKeySize {
        writeJSON(w, resp{OK: false, DID: did, Msg: "bad verkey size"})
        return
    }
    msg := []byte(did + "|" + body.Nonce + "|" + strconv.FormatInt(body.TS, 10))
    sigBytes, err := base64.StdEncoding.DecodeString(body.Sig)
    if err != nil || len(sigBytes) != ed25519.SignatureSize {
        writeJSON(w, resp{OK: false, DID: did, Msg: "bad signature format"})
        return
    }
    if !ed25519.Verify(ed25519.PublicKey(pub), msg, sigBytes) {
        writeJSON(w, resp{OK: false, DID: did, Msg: "signature verify failed"})
        return
    }

    // 3) 幂等：账本里是否已存在？
    if ledgerVK, _ := app.resolveVerkey(did); ledgerVK != nil {
        // 已存在，需与提交的 verkey 一致
        if !equalBytes(ledgerVK, pub) {
            writeJSON(w, resp{OK: false, DID: did, Msg: "DID already on ledger with different verkey"})
            return
        }
        // 一致：继续 Fabric 初始化（或直接成功）
    } else {
        // 4) 写 NYM（通过 Registrar 服务或子进程脚本）
        if err := app.registerNYM(did, verkeyB58, body.Alias); err != nil {
            writeJSON(w, resp{OK: false, DID: did, Msg: "write NYM failed: " + err.Error()})
            return
        }
    }

    // 5) Fabric 账户初始化（幂等）
    txID, err := app.Setup.CreateAccount(did)
    if err != nil {
        writeJSON(w, resp{OK: false, DID: did, Msg: "fabric create account failed: " + err.Error()})
        return
    }
    writeJSON(w, resp{OK: true, DID: did, Msg: "registered", TxID: txID})
}

func equalBytes(a, b []byte) bool {
    if len(a) != len(b) { return false }
    for i := range a { if a[i] != b[i] { return false } }
    return true
}


func (app *Application) registerNYM(did, verkeyB58, alias string) error {
    if app.Registrar == "" {
        return errors.New("Registrar not set")
    }

    // von-browser 的“Register DID”接口
    url := strings.TrimRight(app.Registrar, "/") + "/register"

    // role 要与网页上选择的一致。常见：ENDORSER（旧称 TRUST_ANCHOR）
    payload := map[string]string{
        "did":    did,
        "verkey": verkeyB58,
        "alias":  alias,
        "role":   "ENDORSER",
    }
    b, _ := json.Marshal(payload)

    req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(string(b)))
    req.Header.Set("Content-Type", "application/json")
    client := &http.Client{Timeout: 10 * time.Second}

    resp, err := client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        raw, _ := ioutil.ReadAll(resp.Body)
        return fmt.Errorf("registrar status=%d body=%s", resp.StatusCode, string(raw))
    }
    return nil
}


func (app *Application) didFromAuth(r *http.Request) (string, error) {
    auth := r.Header.Get("Authorization")
    if !strings.HasPrefix(auth, "Bearer ") {
        return "", errors.New("no bearer")
    }
    raw := strings.TrimPrefix(auth, "Bearer ")
    tok, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
        if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, errors.New("unexpected signing")
        }
        return app.JWTSecret, nil
    })
    if err != nil || !tok.Valid {
        return "", errors.New("bad token")
    }
    if claims, ok := tok.Claims.(jwt.MapClaims); ok {
        if sub, ok := claims["sub"].(string); ok && sub != "" {
            return sub, nil
        }
    }
    return "", errors.New("no sub")
}

// GET /api/profile/me  （需要 JWT）
func (app *Application) GetProfileMeHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    did, err := app.didFromAuth(r)
    if err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    if v, ok := app.profiles.Load(did); ok {
        writeJSON(w, v)
        return
    }
    // 没有则返回一个空壳（也可返回 404，看你喜好）
    writeJSON(w, Profile{
        DID:       did,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    })
}

// POST /api/profile  （需要 JWT）—— 新增或更新
func (app *Application) UpsertProfileHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    did, err := app.didFromAuth(r)
    if err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    var in struct {
        DisplayName string `json:"displayName"`
        Email       string `json:"email"`
        Phone       string `json:"phone"`
    }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
        http.Error(w, "bad json", http.StatusBadRequest)
        return
    }
    // 读取旧值，保持 CreatedAt
    now := time.Now()
    prof := Profile{
        DID:         did,
        DisplayName: strings.TrimSpace(in.DisplayName),
        Email:       strings.TrimSpace(in.Email),
        Phone:       strings.TrimSpace(in.Phone),
        CreatedAt:   now,
        UpdatedAt:   now,
    }
    if v, ok := app.profiles.Load(did); ok {
        old := v.(Profile)
        prof.CreatedAt = old.CreatedAt
    }
    app.profiles.Store(did, prof)
    writeJSON(w, map[string]interface{}{
        "ok":     true,
        "profile": prof,
    })
}

// GET /api/profile/names?dids=did1,did2,...
func (app *Application) ProfileNamesHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    auth := r.Header.Get("Authorization")
    if !strings.HasPrefix(auth, "Bearer ") {
        w.WriteHeader(http.StatusUnauthorized)
        _, _ = w.Write([]byte(`{"error":"unauthorized"}`))
        return
    }

    didsCSV := r.URL.Query().Get("dids")
    if didsCSV == "" {
        _ = json.NewEncoder(w).Encode(map[string]string{})
        return
    }
    dids := strings.Split(didsCSV, ",")
    out := make(map[string]string, len(dids))
    for _, d := range dids {
        d = strings.TrimSpace(d)
        if d == "" {
            continue
        }
        if v, ok := app.profiles.Load(d); ok {
            p := v.(Profile)
            if p.DisplayName != "" {
                out[d] = p.DisplayName
                continue
            }
        }
        // 没存过 profile 就先返回空，前端会用 DID 缩写回退
        out[d] = ""
    }
    _ = json.NewEncoder(w).Encode(out)
}
// SeedDisplayName 在启动期注入一个演示 Profile（不做JWT校验）
func (app *Application) SeedDisplayName(did, name string) {
    did = strings.TrimSpace(did)
    name = strings.TrimSpace(name)
    if did == "" || name == "" {
        return
    }
    now := time.Now()
    prof := Profile{
        DID:         did,
        DisplayName: name,
        CreatedAt:   now,
        UpdatedAt:   now,
    }
    app.profiles.Store(did, prof)
}
//工具函数（生成 6 位验证码)
func (app *Application) genOTP(key, did, channel, contact string, ttl time.Duration) string {
    n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
    code := fmt.Sprintf("%06d", n.Int64())
    app.otps.Store(key, otpItem{
        Code: code, Exp: time.Now().Add(ttl),
        DID: did, Channel: channel, Contact: contact,
    })
    log.Printf("[OTP] key=%s code=%s exp=%s\n", key, code, time.Now().Add(ttl))
    return code
}
func (app *Application) verifyOTP(key, code string) bool {
    v, ok := app.otps.Load(key)
    if !ok { return false }
    it := v.(otpItem)
    if time.Now().After(it.Exp) { app.otps.Delete(key); return false }
    if it.Code != strings.TrimSpace(code) { return false }
    app.otps.Delete(key)
    return true
}

//找回 DID：发码 + 校验
// POST /api/recovery/find-did/initiate  body:{ "email": "..."} 或 { "phone": "..." }
func (app *Application) RecoveryFindDIDInitiate(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    var in struct{ Email, Phone string }
    _ = json.NewDecoder(r.Body).Decode(&in)
    contact := strings.TrimSpace(in.Email)
    channel := "email"
    if contact == "" {
        contact = strings.TrimSpace(in.Phone)
        channel = "phone"
    }
    if contact == "" {
        writeJSON(w, map[string]interface{}{"ok": false, "msg":"need email or phone"})
        return
    }

    // 有/无 DID 都返回 ok，避免泄露存在性；真正 DID 在 verify 时返回
    key := channel + ":" + contact
    code := app.genOTP(key, "", channel, contact, 5*time.Minute)

    // 开发期直接把验证码返回；上线后去掉 devCode，换成发邮件/短信
    writeJSON(w, map[string]interface{}{"ok": true, "sent": true, "devCode": code})
}

// POST /api/recovery/find-did/verify  body:{ "email":"...", "phone":"", "code":"123456" }
func (app *Application) RecoveryFindDIDVerify(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    var in struct{ Email, Phone, Code string }
    _ = json.NewDecoder(r.Body).Decode(&in)
    contact := strings.TrimSpace(in.Email)
    channel := "email"
    if contact == "" {
        contact = strings.TrimSpace(in.Phone)
        channel = "phone"
    }
    if contact == "" || strings.TrimSpace(in.Code) == "" {
        writeJSON(w, map[string]interface{}{"ok":false, "msg":"bad request"}); return
    }
    key := channel + ":" + contact
    if !app.verifyOTP(key, in.Code) {
        writeJSON(w, map[string]interface{}{"ok":false, "msg":"otp invalid"}); return
    }

    // 反查 profiles
    var dids []string
    app.profiles.Range(func(k, v interface{}) bool {
        did := k.(string)
        p := v.(Profile)
        if channel=="email" && strings.EqualFold(p.Email, contact) { dids = append(dids, did) }
        if channel=="phone" && p.Phone == contact { dids = append(dids, did) }
        return true
    })
    writeJSON(w, map[string]interface{}{"ok":true, "dids": dids})
}


//种子备份（加密保存）接口
// POST /api/recovery/blob/store  (需要JWT)
// body:{ "cipher": base64, "salt": base64 }
func (app *Application) RecoveryBlobStore(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    did, err := app.didFromAuth(r)
    if err != nil { http.Error(w,"unauthorized", http.StatusUnauthorized); return }

    // 兼容旧字段名（cipher/salt/iv）和新字段名（cipherB64/saltB64/ivB64）
    var in struct {
        CipherB64 string `json:"cipherB64"`
        SaltB64   string `json:"saltB64"`
        IVB64     string `json:"ivB64"`
        Cipher    string `json:"cipher"`
        Salt      string `json:"salt"`
        Iv        string `json:"iv"`
    }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "bad json"}); return
    }
    // 统一取值（新优先，旧兜底）
    c := in.CipherB64; if c == "" { c = in.Cipher }
    s := in.SaltB64;   if s == "" { s = in.Salt }
    iv := in.IVB64;    if iv == "" { iv = in.Iv }
    if c == "" || s == "" || iv == "" {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "missing fields"}); return
    }

    rb := RecoveryBlob{
        DID: did, CipherB64: c, SaltB64: s, IVB64: iv,
        UpdatedAt: time.Now(),
    }
    app.recoveryBlobs.Store(did, rb)
    writeJSON(w, map[string]interface{}{"ok": true, "updatedAt": rb.UpdatedAt.Format(time.RFC3339)})
}


//无登录态下载加密备份（验证码校验）
// POST /api/recovery/blob/request  body:{ "did":"...", "via":"email"|"phone" }
func (app *Application) RecoveryBlobRequest(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    var in struct{ DID, Via string }
    _ = json.NewDecoder(r.Body).Decode(&in)
    did := strings.TrimSpace(in.DID)
    via := strings.ToLower(strings.TrimSpace(in.Via))
    if did=="" || (via!="email" && via!="phone") {
        writeJSON(w, map[string]interface{}{"ok":false, "msg":"bad request"}); return
    }

    // 拿到该 DID 绑定的邮箱/手机
    v, ok := app.profiles.Load(did)
    if !ok { writeJSON(w, map[string]interface{}{"ok":true, "sent": true}) ; return } // 同样避免暴露存在性
    p := v.(Profile)
    var contact string
    if via=="email" { contact = p.Email } else { contact = p.Phone }
    contact = strings.TrimSpace(contact)
    if contact == "" { writeJSON(w, map[string]interface{}{"ok":true, "sent": true}); return }

    key := "did|" + did + "|" + via
    code := app.genOTP(key, did, via, contact, 5*time.Minute)

    writeJSON(w, map[string]interface{}{"ok": true, "sent": true, "devCode": code})
}
func (app *Application) RecoveryBlobGet(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    did, err := app.didFromAuth(r)
    if err != nil { http.Error(w,"unauthorized", http.StatusUnauthorized); return }
    if v, ok := app.recoveryBlobs.Load(did); ok {
        rb := v.(RecoveryBlob)
        writeJSON(w, map[string]interface{}{"ok": true, "blob": rb, "updatedAt": rb.UpdatedAt.Format(time.RFC3339)})
        return
    }
    writeJSON(w, map[string]interface{}{"ok": true, "blob": nil})
}

func (app *Application) RecoveryBlobVerify(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    var in struct{ DID, Via, Code string }
    _ = json.NewDecoder(r.Body).Decode(&in)
    did := strings.TrimSpace(in.DID)
    via := strings.ToLower(strings.TrimSpace(in.Via))
    if did=="" || in.Code=="" || (via!="email" && via!="phone") {
        writeJSON(w, map[string]interface{}{"ok":false, "msg":"bad request"}); return
    }
    key := "did|" + did + "|" + via
    if !app.verifyOTP(key, in.Code) {
        writeJSON(w, map[string]interface{}{"ok":false, "msg":"otp invalid"}); return
    }
    if v, ok := app.recoveryBlobs.Load(did); ok {
        rb := v.(RecoveryBlob)
        writeJSON(w, map[string]interface{}{"ok": true, "blob": rb})
        return
    }
    writeJSON(w, map[string]interface{}{"ok": true, "blob": nil})
}
// POST /api/ticket/batch
// body: { "eventId": "evt_20250301_A", "base": "A", "count": 100, "meta": { "zone":"A区", "price":"199" } }
func (app *Application) TicketBatchHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if app.Ticket == nil {
        http.Error(w, "ticket service not ready", http.StatusInternalServerError)
        return
    }
    var in struct {
        EventID string            `json:"eventId"`
        Base    string            `json:"base"`
        Count   int               `json:"count"`
        Meta    map[string]string `json:"meta"`
    }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
        http.Error(w, "bad json", http.StatusBadRequest)
        return
    }
    if in.EventID == "" || in.Base == "" || in.Count <= 0 {
        http.Error(w, "missing fields", http.StatusBadRequest)
        return
    }
    metaJson, _ := json.Marshal(in.Meta)
    tx, err := app.Ticket.TicketBatchCreate(in.EventID, in.Base, in.Count, string(metaJson))
    if err != nil {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
        return
    }
    writeJSON(w, map[string]interface{}{"ok": true, "txId": tx})
}

// GET /api/ticket?id=...
func (app *Application) TicketGetHandler(w http.ResponseWriter, r *http.Request) {
    id := r.URL.Query().Get("id")
    if id == "" || app.Ticket == nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }
    b, err := app.Ticket.TicketGet(id)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.Write(b)
}

// GET /api/tickets?eventId=...  或  /api/tickets?owner=did
func (app *Application) TicketListHandler(w http.ResponseWriter, r *http.Request) {
    if app.Ticket == nil {
        http.Error(w, "ticket service not ready", http.StatusInternalServerError)
        return
    }
    eventId := r.URL.Query().Get("eventId")
    owner := r.URL.Query().Get("owner")

    var (
        b   []byte
        err error
    )
    if eventId != "" {
        b, err = app.Ticket.TicketListByEvent(eventId)
    } else if owner != "" {
        b, err = app.Ticket.TicketListByOwner(owner)
    } else {
        http.Error(w, "need eventId or owner", http.StatusBadRequest)
        return
    }
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.Write(b)
}

// POST /api/ticket/assign
// body: { "ticketId":"...", "ownerDid":"..." } 
// 若 ownerDid 为空，则默认给当前登录者（JWT 的 sub）
func (app *Application) TicketAssignHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if app.Ticket == nil {
        http.Error(w, "ticket service not ready", http.StatusInternalServerError)
        return
    }
    var in struct {
        TicketID string `json:"ticketId"`
        OwnerDID string `json:"ownerDid"`
    }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
        http.Error(w, "bad json", http.StatusBadRequest)
        return
    }
    if in.TicketID == "" {
        http.Error(w, "ticketId required", http.StatusBadRequest)
        return
    }
    if in.OwnerDID == "" {
        did, err := app.didFromAuth(r)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        in.OwnerDID = did
    }
    tx, err := app.Ticket.TicketAssign(in.TicketID, in.OwnerDID)
    if err != nil {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
        return
    }
    writeJSON(w, map[string]interface{}{"ok": true, "txId": tx})
}

// GET /api/my-tickets   （需要 JWT）
func (app *Application) MyTicketsHandler(w http.ResponseWriter, r *http.Request) {
    if app.Ticket == nil {
        http.Error(w, "ticket service not ready", http.StatusInternalServerError)
        return
    }
    did, err := app.didFromAuth(r)
    if err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    b, err := app.Ticket.TicketListByOwner(did)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.Write(b)
}

// POST /api/ticket/use   （需要 JWT，默认核销自己名下的票，演示版不做核销员角色）
func (app *Application) TicketUseHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if app.Ticket == nil {
        http.Error(w, "ticket service not ready", http.StatusInternalServerError)
        return
    }
    did, err := app.didFromAuth(r)
    if err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    var in struct{ TicketID string `json:"ticketId"` }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.TicketID) == "" {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }
    // 简单校验：票必须属于本人
    b, err := app.Ticket.TicketGet(in.TicketID)
    if err != nil {
        http.Error(w, "ticket not found", http.StatusNotFound)
        return
    }
    var tk struct {
        OwnerDID string `json:"ownerDid"`
        Status   string `json:"status"`
    }
    _ = json.Unmarshal(b, &tk)
    if tk.OwnerDID != did {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }
    tx, err := app.Ticket.TicketMarkUsed(in.TicketID)
    if err != nil {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
        return
    }
    writeJSON(w, map[string]interface{}{"ok": true, "txId": tx})
}

// 提交门票转账（后台高权限，不走 JWT）
func (app *Application) TicketTransferSubmit(w http.ResponseWriter, r *http.Request) {
    r.ParseForm()
    ticketId := r.FormValue("ticketId")
    fromDid  := r.FormValue("fromDid") // 可为空（高权限跳过校验）
    toDid    := r.FormValue("toDid")

    data := &struct {
        TxID string
        Msg  string
        Flag bool
    }{}

    if app.Ticket == nil {
        data.Msg = "ticket service not ready"
        ShowView(w, r, "ticketTransfer.html", data)
        return
    }
    if ticketId == "" || toDid == "" {
        data.Msg = "ticketId / toDid 不能为空"
        ShowView(w, r, "ticketTransfer.html", data)
        return
    }

    txId, err := app.Ticket.TicketTransfer(fromDid, toDid, ticketId)
    if err != nil {
        data.Msg = "转票失败: " + err.Error()
        ShowView(w, r, "ticketTransfer.html", data)
        return
    }
    data.TxID = txId
    data.Flag = true
    data.Msg  = "转票成功！交易ID: " + txId
    ShowView(w, r, "ticketTransfer.html", data)
}

// POST /api/ticket/transfer  { "ticketId":"...", "toDid":"..." }  （需要JWT，校验本人持有）
func (app *Application) TicketTransferAPIHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if app.Ticket == nil {
        http.Error(w, "ticket service not ready", http.StatusInternalServerError)
        return
    }
    did, err := app.didFromAuth(r)
    if err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    var in struct {
        TicketID string `json:"ticketId"`
        ToDid    string `json:"toDid"`
    }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.TicketID) == "" || strings.TrimSpace(in.ToDid) == "" {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }
    // 限制：必须是自己名下的票
    b, err := app.Ticket.TicketGet(in.TicketID)
    if err != nil { writeJSON(w, map[string]interface{}{"ok": false, "msg":"ticket not found"}); return }
    var tk struct{ OwnerDID, Status string }
    _ = json.Unmarshal(b, &tk)
    if tk.OwnerDID != did {
        writeJSON(w, map[string]interface{}{"ok": false, "msg":"forbidden"}); return
    }
    if tk.Status != "issued" {
        writeJSON(w, map[string]interface{}{"ok": false, "msg":"only issued ticket can be transferred"}); return
    }

    tx, err := app.Ticket.TicketTransfer(did, in.ToDid, in.TicketID)
    if err != nil { writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()}); return }
    writeJSON(w, map[string]interface{}{"ok": true, "txId": tx})
}
// GET /gate   —— 检票员网页（无需登录）
func (app *Application) GatePageHandler(w http.ResponseWriter, r *http.Request) {
    // 复用你现有的模板渲染函数
    ShowView(w, r, "gate.html", nil)
}
// POST /api/gate/ticket/use  body:{ "ticketId":"..." } —— 无登录核销（MVP）
func (app *Application) GateTicketUseAPI(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if app.Ticket == nil {
        http.Error(w, "ticket service not ready", http.StatusInternalServerError)
        return
    }

    var in struct{ TicketID string `json:"ticketId"` }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.TicketID) == "" {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    // 预检：存在性 + 状态
    b, err := app.Ticket.TicketGet(in.TicketID)
    if err != nil {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "ticket not found"})
        return
    }
    var tk struct {
        Status  string `json:"status"`
        EventID string `json:"eventId"`
    }
    _ = json.Unmarshal(b, &tk)

    if tk.Status == "used" {
        // 幂等提示
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "ALREADY_USED"})
        return
    }
    if tk.Status != "issued" {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "NOT_ISSUED"})
        return
    }

    // 真正核销（链上修改状态）
    txId, err := app.Ticket.TicketMarkUsed(in.TicketID)
    if err != nil {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
        return
    }
    writeJSON(w, map[string]interface{}{"ok": true, "txId": txId})
}

// POST /api/ticket/claim  { "ticketId":"..." } （需要 JWT）
// 逻辑：
// - 如果票未分配(unassigned) -> TicketAssign 给当前登录 DID
// - 否则若已发放给平台票池(ownerDid==app.PoolDID) -> TicketTransfer("", 当前DID, ticketId)
// - 其他情况拒绝
func (app *Application) TicketClaimHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    if app.Ticket == nil { http.Error(w,"ticket service not ready", http.StatusInternalServerError); return }

    did, err := app.didFromAuth(r)
    if err != nil { http.Error(w,"unauthorized", http.StatusUnauthorized); return }

    var in struct{ TicketID string `json:"ticketId"` }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.TicketID)=="" {
        http.Error(w,"bad request", http.StatusBadRequest); return
    }

    // 查询当前票状态/持有人
    b, err := app.Ticket.TicketGet(in.TicketID)
    if err != nil { writeJSON(w, map[string]interface{}{"ok":false,"msg":"ticket not found"}); return }

    var tk struct {
        OwnerDID string `json:"ownerDid"`
        Status   string `json:"status"`
    }
    _ = json.Unmarshal(b, &tk)

    switch tk.Status {
    case "unassigned":
        // 直接指派给当前登录者
        tx, err := app.Ticket.TicketAssign(in.TicketID, did)
        if err != nil { writeJSON(w, map[string]interface{}{"ok":false,"msg":err.Error()}); return }
        writeJSON(w, map[string]interface{}{"ok":true,"txId":tx})
        return

    case "issued":
        // 如果该票在平台票池名下，走高权限转让（fromDid 传空，链码会跳过校验）
        if app.PoolDID != "" && tk.OwnerDID == app.PoolDID {
            tx, err := app.Ticket.TicketTransfer("", did, in.TicketID)
            if err != nil { writeJSON(w, map[string]interface{}{"ok":false,"msg":err.Error()}); return }
            writeJSON(w, map[string]interface{}{"ok":true,"txId":tx})
            return
        }
        // 否则不可领
        writeJSON(w, map[string]interface{}{"ok":false,"msg":"ticket is not claimable"})
        return

    default:
        writeJSON(w, map[string]interface{}{"ok":false,"msg":"ticket is not claimable"})
        return
    }
}

// POST /api/recovery/passblob/store-pop
// body: { did, nonce, ts, sig, cipherB64, saltB64, ivB64 }
func (app *Application) PassBlobStorePOP(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    var in struct {
        DID       string `json:"did"`
        Nonce     string `json:"nonce"`
        TS        int64  `json:"ts"`
        Sig       string `json:"sig"`
        CipherB64 string `json:"cipherB64"`
        SaltB64   string `json:"saltB64"`
        IVB64     string `json:"ivB64"`
    }
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "bad json"}); return
    }
    did := strings.TrimSpace(in.DID)
    if did=="" || in.Nonce=="" || in.Sig=="" || in.TS==0 || in.CipherB64=="" || in.SaltB64=="" || in.IVB64=="" {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "missing fields"}); return
    }

    // 校验一次性 nonce（与 /auth/nonce 同逻辑）
    v, ok := app.nonces.Load(did)
    if !ok { writeJSON(w, map[string]interface{}{"ok": false, "msg": "nonce not found"}); return }
    it := v.(nonceItem)
    if it.Nonce != in.Nonce || time.Now().After(it.Exp) {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "nonce expired/invalid"}); return
    }
    app.nonces.Delete(did)

    // 用账本解析到的 verkey 验证 sig（did|nonce|ts）
    verkey, err := app.resolveVerkey(did)
    if err != nil { writeJSON(w, map[string]interface{}{"ok": false, "msg": "resolve verkey failed"}); return }
    msg := []byte(did + "|" + in.Nonce + "|" + strconv.FormatInt(in.TS, 10))
    sigBytes, err := base64.StdEncoding.DecodeString(in.Sig)
    if err != nil || len(sigBytes) != ed25519.SignatureSize {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "bad signature format"}); return
    }
    if !ed25519.Verify(verkey, msg, sigBytes) {
        writeJSON(w, map[string]interface{}{"ok": false, "msg": "signature verify failed"}); return
    }

    // 存储（覆盖式，幂等）
    rb := RecoveryBlob{
        DID: did, CipherB64: in.CipherB64, SaltB64: in.SaltB64, IVB64: in.IVB64,
        UpdatedAt: time.Now(),
    }
    app.passBlobs.Store(did, rb)
    writeJSON(w, map[string]interface{}{"ok": true, "updatedAt": rb.UpdatedAt.Format(time.RFC3339)})
}

// GET /api/recovery/passblob/get?did=...
// 返回 { ok:true, blob:{ cipherB64, saltB64, ivB64 } } 或 { ok:true, blob:null }
func (app *Application) PassBlobGetByDID(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet { http.Error(w,"method not allowed", http.StatusMethodNotAllowed); return }
    did := strings.TrimSpace(r.URL.Query().Get("did"))
    if did == "" { http.Error(w, "bad request", http.StatusBadRequest); return }
    if v, ok := app.passBlobs.Load(did); ok {
        rb := v.(RecoveryBlob)
        writeJSON(w, map[string]interface{}{"ok": true, "blob": map[string]string{
            "cipherB64": rb.CipherB64,
            "saltB64":   rb.SaltB64,
            "ivB64":     rb.IVB64,
        }})
        return
    }
    writeJSON(w, map[string]interface{}{"ok": true, "blob": nil})
}


