package requests

import (
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/errors"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/models"
)

type RepositoryRequest interface {
	SaveRequest(request *models.RequestJSON) (int, errors.Err)
	GetRequest(reqId int) (*models.RequestJSON, errors.Err)
	GetRequests(start int, limit int, isFull bool) (models.Requests, errors.Err)
	DeleteRequests() errors.Err
}
