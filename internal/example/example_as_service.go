package example

import (
	"context"
	"errors"
	"fmt"
	"github.com/ing-bank/orchestration-pkg/pkg/orchestration"
)

var _ orchestration.Service = &MemoryApiCreate{}

type MemoryApiCreate struct {
	orchestration.Recoverable // Satisfies the Recover method of the Service interface, but we will never use recovery
	orchestration.Payload     // Adds .Response and implements GetResponse, for easy response generation

	Datacenter string
	Claim      MemoryClaim
	ModifiedDb bool // To make sure Rollback only modifies the DB when this Service changed it
}

func (c *MemoryApiCreate) Name() string {
	return "MyService Create " + c.Datacenter
}

func (c *MemoryApiCreate) Check(_ context.Context) error {
	c.Claim.ClaimName = c.Datacenter + c.Claim.ClaimName // Make name datacenter unique

	if _, ok := FakeDbRead(c.Claim.ClaimName); ok {
		return errors.New("already exists")
	}

	if available := FakeDbGetAvailableMemory(); available-c.Claim.MemoryInMb < 0 {
		return errors.New(fmt.Sprintf("not enough memory available: (%d/%d)", c.Claim.MemoryInMb, available))
	}

	return nil
}

func (c *MemoryApiCreate) Run(_ context.Context) error {
	FakeDbUpdateAvailableMemory(-c.Claim.MemoryInMb)
	FakeDbWrite(c.Claim)
	c.Response = "ok"
	c.ModifiedDb = true

	// We still need to check if available memory has been breached
	// In this example we have one dummy memory counter for 4 Datacenters/Zones, which is obviously a big issue
	if FakeDbGetAvailableMemory() < 0 {
		return errors.New(fmt.Sprintf("not enough memory available"))
	}
	return nil
}

func (c *MemoryApiCreate) Rollback(_ context.Context) error {
	if c.ModifiedDb {
		FakeDbDelete(c.Claim.ClaimName)
		FakeDbUpdateAvailableMemory(c.Claim.MemoryInMb)
	}
	return nil
}
