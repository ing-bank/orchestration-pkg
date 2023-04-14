package orchestration

import (
	"net/http"
)

type Response struct {
	Status  string           `json:"status"`
	Details []ResponseDetail `json:"details"`
}

type ResponseDetail struct {
	Name   string      `json:"name"`
	Detail interface{} `json:"detail"`
}

type Payload struct {
	Response interface{}
}

func (p *Payload) GetResponse(err error) interface{} {
	if err != nil {
		return err.Error()
	}
	if p.Response == nil {
		return "ok"
	}
	return p.Response
}

func generateResponseContainer(err error) (int, *Response) {
	response := &Response{Status: "ok"}
	status := http.StatusOK

	if err != nil {
		response.Status = err.Error()
		status = http.StatusInternalServerError
	}

	return status, response
}

func GenerateResponse(services []Service, errs []error, err error) (int, *Response) {
	status, response := generateResponseContainer(err)

	for i, service := range services {
		detail := service.GetResponse(errs[i])
		if detail != nil {
			response.Details = append(response.Details, ResponseDetail{
				Name:   service.Name(),
				Detail: detail,
			})
		}
	}

	return status, response
}

func GenerateStagedResponse(stages [][]Service, failedStageIndex int, errs []error, err error) (int, *Response) {
	if err != nil {
		return GenerateResponse(stages[failedStageIndex], errs, err)
	}

	response := &Response{Status: "ok"}
	status := http.StatusOK
	for _, stage := range stages {
		_, stageResponse := GenerateResponse(stage, make([]error, len(stage)), nil)
		response.Details = append(response.Details, stageResponse.Details...)
	}

	return status, response
}
