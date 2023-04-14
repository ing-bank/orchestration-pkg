package main

import (
	"context"
	"encoding/json"
	"github.com/ing-bank/orchestration-pkg/internal/example"
	"github.com/ing-bank/orchestration-pkg/pkg/orchestration"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/api/v1/memory", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			// Parse JSON request into MemoryClaim
			claim := example.MemoryClaim{}
			if err := json.NewDecoder(request.Body).Decode(&claim); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = writer.Write([]byte("could not unmarshal request payload: " + err.Error()))
			}
			CopyClaim := func(claim example.MemoryClaim) *example.MemoryClaim {
				return &claim
			}

			// Create memory API claims in many datacenters/zones as an example
			services := []orchestration.Service{
				orchestration.RestApiAsService(&example.MyServiceApi{Datacenter: "DC1_BLUE"},
					orchestration.REST_API_POST, "MyService Create DC1_BLUE", "", CopyClaim(claim)),
				orchestration.RestApiAsService(&example.MyServiceApi{Datacenter: "DC1_RED"},
					orchestration.REST_API_POST, "MyService Create DC1_RED", "", CopyClaim(claim)),
				orchestration.RestApiAsService(&example.MyServiceApi{Datacenter: "DC2_BLUE"},
					orchestration.REST_API_POST, "MyService Create DC2_BLUE", "", CopyClaim(claim)),
				orchestration.RestApiAsService(&example.MyServiceApi{Datacenter: "DC2_RED"},
					orchestration.REST_API_POST, "MyService Create DC2_RED", "", CopyClaim(claim)),
			}
			errs, err := orchestration.CallServices(context.TODO(), services, orchestration.CallServicesOpts{}) // Calls: Check -> Run -> Rollback

			// Generate response
			status, resp := orchestration.GenerateResponse(services, errs, err)
			writer.WriteHeader(status)
			rawResp, _ := json.Marshal(resp)
			_, _ = writer.Write(append(rawResp, '\n'))

		} else {
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// When a Service has a Rollback it is executed "in the background". Since a rollback is a fallible
	// operation the error needs to be reported somewhere. The orchestration package calls the RollbackErrorReporter
	// with all the services in a stage when a Service Rollback has an error.
	orchestration.RollbackErrorReporter = func(_ context.Context, services []orchestration.Service, errs []error) {
		for i := 0; i < len(services); i++ {
			if errs[i] != nil {
				log.Printf("Rollback failed for Service %s: %v", services[i].Name(), errs[i])
			}
		}
	}

	log.Fatal(http.ListenAndServe(":8090", nil))
}
