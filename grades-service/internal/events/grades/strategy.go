package grades

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ilyadubrovsky/bars"
	"grades-service/internal/events/model"
)

type Service interface {
	GetProgressTableFromDB(ctx context.Context, id int64) (*bars.ProgressTable, error)
	GetProgressTableByRequest(ctx context.Context, id int64) (*bars.ProgressTable, error)
}

type ProcessStrategy struct {
	service         Service
	unavailablePT   string
	notAuthorized   string
	botError        string
	ptNotProvided   string
	wrongGradesPage string
}

func (s *ProcessStrategy) Process(body []byte) (*model.GetGradesResponse, error) {
	var (
		request GetGradesRequest
	)
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("json unmarshal: %v", err)
	}

	response := &model.GetGradesResponse{
		RequestID:    request.RequestID,
		IsCallback:   request.IsCallback,
		CallbackData: request.CallbackData,
		MessageID:    request.MessageID,
	}

	progressTable, err := s.service.GetProgressTableFromDB(context.Background(), request.RequestID)
	if err != nil {
		response.ResponseMessage = s.botError
		return response, fmt.Errorf("service: %v", err)
	}

	if progressTable == nil {
		response.ResponseMessage = s.notAuthorized
	} else if len(progressTable.Tables) == 0 {
		pt, err := s.service.GetProgressTableByRequest(context.Background(), request.RequestID)
		if err != nil {
			switch err {
			case bars.ErrWrongGradesPage:
				response.ResponseMessage = s.wrongGradesPage
			case bars.ErrNoAuth:
				response.ResponseMessage = s.notAuthorized
			default:
				response.ResponseMessage = s.botError
				return response, fmt.Errorf("service: %v", err)
			}
			return response, nil
		}
		if pt == nil {
			response.ResponseMessage = s.notAuthorized
		} else if len(pt.Tables) == 0 {
			response.ResponseMessage = s.ptNotProvided
		} else {
			response.ProgressTable = *pt
		}
	} else {
		response.ProgressTable = *progressTable
	}

	return response, nil
}

func NewProcessStrategy(service Service, notAuthorized, botError, ptNotProvided, wrongGradesPage string) *ProcessStrategy {
	return &ProcessStrategy{service: service, notAuthorized: notAuthorized,
		botError: botError, ptNotProvided: ptNotProvided, wrongGradesPage: wrongGradesPage}
}