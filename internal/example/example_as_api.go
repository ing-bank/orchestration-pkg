package example

import (
	"context"
	"errors"
	"github.com/ing-bank/orchestration-pkg/pkg/orchestration"
)

var _ orchestration.RestApi = &MyServiceApi{}

type MyServiceApi struct {
	Datacenter string
}

func (m *MyServiceApi) Get(ctx context.Context, name string) (orchestration.Nameable, error) {
	if claim, ok := FakeDbRead(m.Datacenter + name); ok {
		return &claim, nil
	}
	return nil, errors.New("not found")
}

func (m *MyServiceApi) Post(ctx context.Context, obj orchestration.Nameable) (interface{}, error) {
	rawClaim, ok := obj.(*MemoryClaim)
	if !ok {
		return nil, errors.New("not a memory claim")
	}
	claim := *rawClaim
	claim.ClaimName = m.Datacenter + claim.ClaimName

	_, ok = FakeDbRead(claim.ClaimName)
	if ok {
		return nil, errors.New("already exists")
	}

	if FakeDbGetAvailableMemory()-claim.MemoryInMb < 0 {
		return nil, errors.New("not enough memory available")
	}

	FakeDbWrite(claim)
	FakeDbUpdateAvailableMemory(-claim.MemoryInMb)

	if FakeDbGetAvailableMemory() < 0 { // Can happen due to concurrency issues
		return nil, errors.New("not enough memory available after claiming")
	}

	return "ok", nil
}

func (m *MyServiceApi) Put(ctx context.Context, obj orchestration.Nameable) (interface{}, error) {
	rawClaim, ok := obj.(*MemoryClaim)
	if !ok {
		return nil, errors.New("not a memory claim")
	}
	claim := *rawClaim
	claim.ClaimName = m.Datacenter + claim.ClaimName

	existingClaim, ok := FakeDbRead(claim.ClaimName)
	if !ok {
		return nil, errors.New("not found")
	}

	if FakeDbGetAvailableMemory()+existingClaim.MemoryInMb-claim.MemoryInMb < 0 {
		return nil, errors.New("not enough memory available")
	}

	FakeDbWrite(claim)
	FakeDbUpdateAvailableMemory(existingClaim.MemoryInMb - claim.MemoryInMb)
	return "ok", nil
}

func (m *MyServiceApi) Delete(ctx context.Context, name string) (interface{}, error) {
	name = m.Datacenter + name
	claim, ok := FakeDbRead(name)
	if !ok {
		return nil, errors.New("not found")
	}
	FakeDbDelete(name)
	FakeDbUpdateAvailableMemory(claim.MemoryInMb)
	return "ok", nil
}

func (m *MyServiceApi) List(ctx context.Context) (interface{}, error) {
	return FakeDbList(), nil
}
