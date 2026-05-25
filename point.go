package main

import (
	"encoding/json"
	"fmt"
	"strconv"
//	"bytes"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/peer"
)

const (
	DOC_TYPE     = "accountObj"
	COMPANY_ID   = "company_sldp"
	INIT_BALANCE = 66000000000000
)

type Account struct {
	ObjectType string `json:"docType"`
	ID         string `json:"id"`       // 用户 ID（Indy DID）
	Balance    uint64 `json:"balance"`  // 当前积分余额
}

type HistoryItem struct {
	TxId    string  `json:"txId"`
	Timestamp int64   `json:"Timestamp"`
	Account Account `json:"account"`
}

type PointChaincode struct{}

func (t *PointChaincode) Init(stub shim.ChaincodeStubInterface) peer.Response {
	fmt.Println("==== Init Ledger ====")
	// 初始化公司账户，发放初始积分
	company := Account{
		ObjectType: DOC_TYPE,
		ID:         COMPANY_ID,
		Balance:    INIT_BALANCE,
	}
	b, err := json.Marshal(company)
	if err != nil {
		return shim.Error("公司账户序列化失败")
	}
	err = stub.PutState(company.ID, b)
	if err != nil {
		return shim.Error("保存公司账户失败")
	}
	return shim.Success([]byte("初始化成功"))
}

func (t *PointChaincode) Invoke(stub shim.ChaincodeStubInterface) peer.Response {
	fun, args := stub.GetFunctionAndParameters()

	switch fun {
	case "CreateAccount":
		return t.CreateAccount(stub, args)
	case "Mint":
		return t.Mint(stub, args)
	case "Transfer":
		return t.Transfer(stub, args)
	case "GetBalance":
		return t.GetBalance(stub, args)
	case "GetHistory":
		return t.GetHistory(stub, args)
	case "GetAllAccounts":
		return t.GetAllAccounts(stub)
	default:
		return shim.Error("无效的方法名: " + fun)
	}
}

// PutAccount 保存账户
func PutAccount(stub shim.ChaincodeStubInterface, acc Account) error {
	acc.ObjectType = DOC_TYPE
	b, err := json.Marshal(acc)
	if err != nil {
		return err
	}
	return stub.PutState(acc.ID, b)
}

// GetAccount 读取账户
func GetAccount(stub shim.ChaincodeStubInterface, id string) (Account, error) {
	var acc Account
	b, err := stub.GetState(id)
	if err != nil {
		return acc, err
	}
	if b == nil {
		return acc, fmt.Errorf("账户不存在: %s", id)
	}
	err = json.Unmarshal(b, &acc)
	return acc, err
}

// CreateAccount 创建账户
// args: args[0]=accountJson, args[1]=eventName
func (t *PointChaincode) CreateAccount(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	if len(args) != 2 {
		return shim.Error("参数数量错误")
	}

	var acc Account
	err := json.Unmarshal([]byte(args[0]), &acc)
	if err != nil {
		return shim.Error("账户信息解析失败")
	}

	_, err = GetAccount(stub, acc.ID)
	if err == nil {
		return shim.Error("账户已存在")
	}

	acc.Balance = 0
	err = PutAccount(stub, acc)
	if err != nil {
		return shim.Error("账户保存失败")
	}

	stub.SetEvent(args[1], []byte{})
	return shim.Success([]byte("账户创建成功"))
}

// Mint 发放积分（公司 → 用户）
// args: args[0]=toID, args[1]=amount, args[2]=eventName
func (t *PointChaincode) Mint(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	if len(args) != 3 {
		return shim.Error("参数数量错误")
	}

	toID := args[0]
	amount, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return shim.Error("积分数量格式错误")
	}

	fromAcc, err := GetAccount(stub, COMPANY_ID)
	if err != nil {
		return shim.Error("公司账户不存在")
	}

	toAcc, err := GetAccount(stub, toID)
	if err != nil {
		return shim.Error("目标账户不存在")
	}

	if fromAcc.Balance < amount {
		return shim.Error("公司账户余额不足")
	}

	fromAcc.Balance -= amount
	toAcc.Balance += amount

	PutAccount(stub, fromAcc)
	PutAccount(stub, toAcc)

	stub.SetEvent(args[2], []byte{})
	return shim.Success([]byte("发放成功"))
}

// Transfer 转账（用户之间）
// args: args[0]=fromID, args[1]=toID, args[2]=amount, args[3]=eventName
func (t *PointChaincode) Transfer(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	if len(args) != 4 {
		return shim.Error("参数数量错误")
	}

	fromID := args[0]
	toID := args[1]
	amount, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		return shim.Error("积分数量格式错误")
	}

	fromAcc, err := GetAccount(stub, fromID)
	if err != nil {
		return shim.Error("来源账户不存在")
	}

	toAcc, err := GetAccount(stub, toID)
	if err != nil {
		return shim.Error("目标账户不存在")
	}

	if fromAcc.Balance < amount {
		return shim.Error("余额不足")
	}

	fromAcc.Balance -= amount
	toAcc.Balance += amount

	PutAccount(stub, fromAcc)
	PutAccount(stub, toAcc)

	stub.SetEvent(args[3], []byte{})
	return shim.Success([]byte("转账成功"))
}

// GetBalance 查询余额
// args: args[0]=userID
func (t *PointChaincode) GetBalance(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	if len(args) != 1 {
		return shim.Error("参数数量错误")
	}

	acc, err := GetAccount(stub, args[0])
	if err != nil {
		return shim.Error("账户读取失败")
	}

	b, _ := json.Marshal(acc)
	return shim.Success(b)
}

// GetHistory 查询账户历史
// args: args[0]=userID
func (t *PointChaincode) GetHistory(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	if len(args) != 1 {
		return shim.Error("参数数量错误")
	}

	id := args[0]
	iter, err := stub.GetHistoryForKey(id)
	if err != nil {
		return shim.Error("查询历史失败")
	}
	defer iter.Close()

	var history []HistoryItem
	for iter.HasNext() {
		tx, err := iter.Next()
		if err != nil {
			return shim.Error("读取历史失败")
		}

		var acc Account
		if tx.Value != nil {
			json.Unmarshal(tx.Value, &acc)
		}

		history = append(history, HistoryItem{
			TxId:    tx.TxId,
			Timestamp: tx.Timestamp.Seconds, 
			Account: acc,
		})
	}

	b, _ := json.Marshal(history)
	return shim.Success(b)
}

// GetAllAccounts 获取所有账户信息
func (t *PointChaincode) GetAllAccounts(stub shim.ChaincodeStubInterface) peer.Response {
	iter, err := stub.GetStateByRange("", "")
	if err != nil {
		return shim.Error("查询所有账户失败")
	}
	defer iter.Close()

	var accounts []Account
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return shim.Error("遍历账户失败")
		}
		var acc Account
		json.Unmarshal(kv.Value, &acc)
		if acc.ObjectType == DOC_TYPE && acc.ID != "company_sldp" { // 排除公司账户
			accounts = append(accounts, acc)
		}
	}

	b, _ := json.Marshal(accounts)
	return shim.Success(b)
}


func main() {
	err := shim.Start(new(PointChaincode))
	if err != nil {
		fmt.Printf("启动PointChaincode失败: %s", err)
	}
}

