package service

import (
	"encoding/json"
	"strconv"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
)

// 新 InitAccount 调整为 CreateAccount 调用
func (t *ServiceSetup) CreateAccount(userID string) (string, error) {
	eventID := "eventCreateAccount"
	reg, notifier := regitserEvent(t.Client, t.ChaincodeID, eventID)
	defer t.Client.UnregisterChaincodeEvent(reg)

	// 1) 组装账户对象（初始余额为 0）
	account := struct {
		ObjectType string `json:"docType"`
		ID         string `json:"id"`
		Balance    uint64 `json:"balance"`
	}{
		ObjectType: "accountObj",
		ID:         userID,
		Balance:    0,
	}

	// 2) 调用链码创建账户并等待事件，确保账户已提交到账本
	accBytes, _ := json.Marshal(account)
	req := channel.Request{
		ChaincodeID: t.ChaincodeID,
		Fcn:         "CreateAccount",
		Args:        [][]byte{accBytes, []byte(eventID)},
	}
	resp, err := t.Client.Execute(req)
	if err != nil {
		return "", err
	}
	if err := eventResult(notifier, eventID); err != nil {
		return "", err
	}

	// 3) 从公司总账转 100 分给新账户（使用 Transfer，而非 Mint）
	const companyID = "company_sldp"
	const welcomeBonus uint64 = 100

	// 这里复用你已实现的 Transfer（其内部已注册并等待事件）
	if _, err := t.Transfer(companyID, userID, welcomeBonus); err != nil {
		// 账户已创建，但转账失败；上层可按需补转或记录告警
		return "", err
	}

	// 保持与原实现一致，返回“创建账户”这笔交易的 TxID
	return string(resp.TransactionID), nil
}

func (t *ServiceSetup) GetBalance(userID string) ([]byte, error) {
	req := channel.Request{ChaincodeID: t.ChaincodeID, Fcn: "GetBalance", Args: [][]byte{[]byte(userID)}}
	respone, err := t.Client.Query(req)
	if err != nil {
		return nil, err
	}
	return respone.Payload, nil
}

func (t *ServiceSetup) Transfer(fromID, toID string, amount uint64) (string, error) {
	eventID := "eventTransfer"
	reg, notifier := regitserEvent(t.Client, t.ChaincodeID, eventID)
	defer t.Client.UnregisterChaincodeEvent(reg)

	amountStr := strconv.FormatUint(amount, 10)
	req := channel.Request{
		ChaincodeID: t.ChaincodeID,
		Fcn:        "Transfer",
		Args:       [][]byte{[]byte(fromID), []byte(toID), []byte(amountStr), []byte(eventID)},
	}
	respone, err := t.Client.Execute(req)
	if err != nil {
		return "", err
	}
	err = eventResult(notifier, eventID)
	if err != nil {
		return "", err
	}
	return string(respone.TransactionID), nil
}



func (t *ServiceSetup) Mint(toID string, amount uint64) (string, error) {
	eventID := "eventMint"
	reg, notifier := regitserEvent(t.Client, t.ChaincodeID, eventID)
	defer t.Client.UnregisterChaincodeEvent(reg)

	amountStr := strconv.FormatUint(amount, 10)
	req := channel.Request{
		ChaincodeID: t.ChaincodeID,
		Fcn:        "Mint",
		Args:       [][]byte{[]byte(toID), []byte(amountStr), []byte(eventID)},
	}
	respone, err := t.Client.Execute(req)
	if err != nil {
		return "", err
	}
	err = eventResult(notifier, eventID)
	if err != nil {
		return "", err
	}
	return string(respone.TransactionID), nil
}

// GetAllAccounts 查询所有用户账户数据（用于排行榜）
func (t *ServiceSetup) GetAllAccounts() ([]byte, error) {
	req := channel.Request{
		ChaincodeID: t.ChaincodeID,
		Fcn:         "GetAllAccounts",
		Args:        [][]byte{},
	}
	response, err := t.Client.Query(req)
	if err != nil {
		return nil, err
	}
	return response.Payload, nil
}
func (t *ServiceSetup) GetHistory(userID string) ([]byte, error) {
    req := channel.Request{
        ChaincodeID: t.ChaincodeID,
        Fcn:         "GetHistory",
        Args:        [][]byte{[]byte(userID)},
    }

    response, err := t.Client.Query(req)
    if err != nil {
        return nil, err
    }

    return response.Payload, nil
}

