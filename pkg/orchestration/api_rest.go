package orchestration

import (
	"context"
	"errors"
	"net/http"
)

type RestApi interface {
	Get(ctx context.Context, name string) (Nameable, error)
	Post(ctx context.Context, obj Nameable) (interface{}, error)
	Put(ctx context.Context, obj Nameable) (interface{}, error)
	Delete(ctx context.Context, name string) (interface{}, error)

	List(ctx context.Context) (interface{}, error)
}

// Nameable presents an object that has a (unique) name
type Nameable interface {
	Name() string
}

type RestApiAction string

const (
	REST_API_GET    RestApiAction = http.MethodGet
	REST_API_POST   RestApiAction = http.MethodPost
	REST_API_PUT    RestApiAction = http.MethodPut
	REST_API_DELETE RestApiAction = http.MethodDelete
	REST_API_LIST   RestApiAction = "LIST" // Not an HTTP standard
)

var _ Service = &SimpleRestApiService{}

// SimpleRestApiService converts a Rest API to a Service without Check and Rollback
type SimpleRestApiService struct {
	Recoverable

	ApiName        string
	Api            RestApi       // Always required
	Action         RestApiAction // Always Required, selects what action the Service should provide
	RequestName    string        // Required when Action is GET, DELETE
	RequestPayload Nameable      // Required when Action is POST, PUT

	Response interface{} // Rest API response will be stored here when calling Run
}

// RestApiService converts a Rest API to a Service with inferred Check and Rollback
type RestApiService struct {
	SimpleRestApiService
	backup Nameable
}

func RestApiAsService(api RestApi, action RestApiAction, apiName, name string, payload Nameable) Service {
	return &RestApiService{
		SimpleRestApiService: SimpleRestApiService{
			Api:            api,
			ApiName:        apiName,
			Action:         action,
			RequestName:    name,
			RequestPayload: payload,
		},
	}
}

func (proto *SimpleRestApiService) Name() string {
	return proto.ApiName
}

func (proto *SimpleRestApiService) GetResponse(err error) interface{} {
	if err != nil {
		return err.Error()
	}
	return proto.Response
}

func (proto *SimpleRestApiService) Check(_ context.Context) error {
	return nil
}

func (proto *SimpleRestApiService) Run(ctx context.Context) error {
	var response interface{}
	var err error

	if proto.Action == REST_API_GET {
		response, err = proto.Api.Get(ctx, proto.RequestName)
	} else if proto.Action == REST_API_POST {
		response, err = proto.Api.Post(ctx, proto.RequestPayload)
	} else if proto.Action == REST_API_PUT {
		response, err = proto.Api.Put(ctx, proto.RequestPayload)
	} else if proto.Action == REST_API_DELETE {
		response, err = proto.Api.Delete(ctx, proto.RequestName)
	} else if proto.Action == REST_API_LIST {
		response, err = proto.Api.List(ctx)
	} else {
		return errors.New("unrecognized RestApiService Action (did you init?): " + string(proto.Action))
	}

	proto.Response = response
	return err
}

func (proto *SimpleRestApiService) Rollback(_ context.Context) error {
	return nil
}

func (proto *RestApiService) Check(ctx context.Context) error {
	if proto.Action == REST_API_GET || proto.Action == REST_API_LIST {
		return nil // No Check required
	}

	backup, err := proto.Api.Get(ctx, proto.RequestName)

	if proto.Action == REST_API_POST {
		if err == nil {
			return errors.New("cannot create " + proto.RequestName + " because it already exists")
		}
		return nil
	}

	proto.backup = backup
	return err
}

func (proto *RestApiService) Rollback(ctx context.Context) error {
	if proto.Action == REST_API_PUT {
		// In case Update failed, we Update again to restore backup
		_, err := proto.Api.Put(ctx, proto.backup)
		return err
	}

	if proto.Action == REST_API_POST {
		// In case Creation failed, we Delete
		name := proto.RequestPayload.Name()
		_, err := proto.Api.Delete(ctx, name)
		return err
	}

	if proto.Action == REST_API_DELETE {
		// In case of Deletion failed, we (re-)Create from backup
		_, err := proto.Api.Post(ctx, proto.backup)
		return err
	}

	return nil // Nothing to rollback for Get/List
}
