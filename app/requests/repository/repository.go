package repository

import (
	"encoding/json"
	"fmt"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/errors"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/models"
	"github.com/H-b-IO-T-O-H/proxy-server/app/requests"
	"gorm.io/gorm"
	"net/http"
)

type pgStorage struct {
	db *gorm.DB
}

func (p pgStorage) DeleteRequests() errors.Err {
	if err := p.db.Exec("delete from public.requests; alter sequence public.requests_req_id_seq restart with 1").Error; err != nil {
		return errors.RespErr{Status: http.StatusNotFound, Message: fmt.Sprintf("Не удалось очистить БД: %v", err)}
	}
	return nil
}

func (p pgStorage) SaveRequest(request *models.RequestJSON) (int, errors.Err) {
	if err := p.db.Create(request).Error; err != nil {
		return 0, errors.RespErr{Status: 500, Message: err.Error()}
	}
	return request.ID, nil
}

func (p pgStorage) GetRequest(reqId int) (*models.RequestJSON, errors.Err) {
	var buf models.RequestJSON

	buf.ID = reqId
	if err := p.db.Take(&buf).Error; err != nil {
		if errors.NoRows(err.Error()) {
			return nil, errors.RespErr{Status: http.StatusNotFound, Message: errors.NotFound}
		}
		return nil, errors.RespErr{Status: 500, Message: err.Error()}
	}
	if err := json.Unmarshal(buf.RegByte, &buf.Req); err != nil {
		return nil, errors.RespErr{Status: 500, Message: err.Error()}
	}
	return &buf, nil
}

func (p pgStorage) GetRequests(start int, limit int, isFull bool) (models.Requests, errors.Err) {
	var buf models.Requests

	if err := p.db.Limit(limit).Offset(start).Find(&buf).Error; err != nil {
		return nil, errors.RespErr{Status: 500, Message: err.Error()}
	}
	if isFull {
		for _, req := range buf {
			if err := json.Unmarshal(req.RegByte, &req.Req); err != nil {
				return nil, errors.RespErr{Status: 500, Message: err.Error()}
			}
		}
	}
	return buf, nil
}

func NewPgRepository(db *gorm.DB) requests.RepositoryRequest {
	return &pgStorage{db: db}
}
