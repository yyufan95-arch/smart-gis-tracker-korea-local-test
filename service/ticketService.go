// service/ticketService.go
package service

import (
	"encoding/json"
	"strconv"

	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
)

type TicketService struct {
	Client      *channel.Client
	ChaincodeID string // e.g. "ticketcc"
}

// 创建单张
func (s *TicketService) TicketCreate(ticket interface{}) (string, error) {
	eventID := "eventTicketCreate"
	reg, notifier := regitserEvent(s.Client, s.ChaincodeID, eventID)
	defer s.Client.UnregisterChaincodeEvent(reg)

	b, _ := json.Marshal(ticket)
	req := channel.Request{
		ChaincodeID: s.ChaincodeID,
		Fcn:         "TicketCreate",
		Args:        [][]byte{b, []byte(eventID)},
	}
	resp, err := s.Client.Execute(req)
	if err != nil {
		return "", err
	}
	if err := eventResult(notifier, eventID); err != nil {
		return "", err
	}
	return string(resp.TransactionID), nil
}

// 批量创建
func (s *TicketService) TicketBatchCreate(eventId, base string, count int, metaJson string) (string, error) {
	eventID := "eventTicketBatchCreate"
	reg, notifier := regitserEvent(s.Client, s.ChaincodeID, eventID)
	defer s.Client.UnregisterChaincodeEvent(reg)

	req := channel.Request{
		ChaincodeID: s.ChaincodeID,
		Fcn:         "TicketBatchCreate",
		Args: [][]byte{
			[]byte(eventId),
			[]byte(base),
			[]byte(strconv.Itoa(count)),
			[]byte(eventID),
			[]byte(metaJson),
		},
	}
	resp, err := s.Client.Execute(req)
	if err != nil {
		return "", err
	}
	if err := eventResult(notifier, eventID); err != nil {
		return "", err
	}
	return string(resp.TransactionID), nil
}

func (s *TicketService) TicketGet(ticketId string) ([]byte, error) {
	req := channel.Request{ChaincodeID: s.ChaincodeID, Fcn: "TicketGet", Args: [][]byte{[]byte(ticketId)}}
	r, err := s.Client.Query(req)
	if err != nil {
		return nil, err
	}
	return r.Payload, nil
}

func (s *TicketService) TicketListByEvent(eventId string) ([]byte, error) {
	req := channel.Request{ChaincodeID: s.ChaincodeID, Fcn: "TicketListByEvent", Args: [][]byte{[]byte(eventId)}}
	r, err := s.Client.Query(req)
	if err != nil {
		return nil, err
	}
	return r.Payload, nil
}

func (s *TicketService) TicketListByOwner(ownerDid string) ([]byte, error) {
	req := channel.Request{ChaincodeID: s.ChaincodeID, Fcn: "TicketListByOwner", Args: [][]byte{[]byte(ownerDid)}}
	r, err := s.Client.Query(req)
	if err != nil {
		return nil, err
	}
	return r.Payload, nil
}

func (s *TicketService) TicketAssign(ticketId, toDid string) (string, error) {
	eventID := "eventTicketAssign"
	reg, notifier := regitserEvent(s.Client, s.ChaincodeID, eventID)
	defer s.Client.UnregisterChaincodeEvent(reg)

	req := channel.Request{
		ChaincodeID: s.ChaincodeID,
		Fcn:         "TicketAssign",
		Args:        [][]byte{[]byte(ticketId), []byte(toDid), []byte(eventID)},
	}
	resp, err := s.Client.Execute(req)
	if err != nil {
		return "", err
	}
	if err := eventResult(notifier, eventID); err != nil {
		return "", err
	}
	return string(resp.TransactionID), nil
}

func (s *TicketService) TicketMarkUsed(ticketId string) (string, error) {
	eventID := "eventTicketMarkUsed"
	reg, notifier := regitserEvent(s.Client, s.ChaincodeID, eventID)
	defer s.Client.UnregisterChaincodeEvent(reg)

	req := channel.Request{
		ChaincodeID: s.ChaincodeID,
		Fcn:         "TicketMarkUsed",
		Args:        [][]byte{[]byte(ticketId), []byte(eventID)},
	}
	resp, err := s.Client.Execute(req)
	if err != nil {
		return "", err
	}
	if err := eventResult(notifier, eventID); err != nil {
		return "", err
	}
	return string(resp.TransactionID), nil
}

func (s *TicketService) TicketTransfer(fromDid, toDid, ticketId string) (string, error) {
    eventID := "eventTicketTransfer"
    reg, notifier := regitserEvent(s.Client, s.ChaincodeID, eventID)
    defer s.Client.UnregisterChaincodeEvent(reg)

    req := channel.Request{
        ChaincodeID: s.ChaincodeID,
        Fcn:         "TicketTransfer",
        Args:        [][]byte{[]byte(fromDid), []byte(toDid), []byte(ticketId), []byte(eventID)},
    }
    resp, err := s.Client.Execute(req)
    if err != nil {
        return "", err
    }
    if err := eventResult(notifier, eventID); err != nil {
        return "", err
    }
    return string(resp.TransactionID), nil
}

