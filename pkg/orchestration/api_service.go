package orchestration

import (
	"context"
	"errors"
	"github.com/ing-bank/orchestration-pkg/pkg/task"
	"log"
	"strings"
)

type Service interface {
	Nameable

	Check(ctx context.Context) error
	Recover(ctx context.Context) error // Advanced usage, inherit from Recoverable to ignore
	Run(ctx context.Context) error
	Rollback(ctx context.Context) error

	GetResponse(err error) any
}

type CallServicesOpts struct {
	SkipRollback  bool
	OnActionError func(ctx context.Context, action ServiceAction, services []Service, errs []error) // Check/Run actions

	OnStageStart func(ctx context.Context, services []Service)
}

// SimpleService implements, Check, Recover, Rollback, and GetResponse with dummy implementations
type SimpleService struct {
	Responder
	Recoverable
}

func (s SimpleService) Check(ctx context.Context) error {
	return nil
}

func (s SimpleService) Rollback(ctx context.Context) error {
	return errors.New("rollback not implemented")
}

type Responder struct {
	Response                 any
	UseNullResponseAsDefault bool
}

func (r *Responder) GetResponse(err error) any {
	if err != nil {
		return err.Error()
	}
	if !r.UseNullResponseAsDefault && r.Response == nil {
		return "ok"
	}
	return r.Response
}

// Recoverable allows a Recovery func to be set, which is called when recover is set and there is an error during
// the check stage of this specific Service
type Recoverable struct {
	Recovery func(ctx context.Context) error
}

// DryRunService wraps a given Service and overrides the Run/Rollback functions as nil
// Mainly used in staged Services, where a dry run of a call is required and a Run will
// be called in a later stage.
type DryRunService struct {
	Recoverable
	Wrapper Service
}

func MakeDryRun(service Service) Service {
	return &DryRunService{Wrapper: service}
}

// RollbackErrorReporter is called when a SERVICE_ROLLBACK action results in one or more errors
// E.g. can be used to create incidents, or other reporting. Called concurrently.
var RollbackErrorReporter func(context.Context, []Service, []error)

var ActionLogger = func(_ context.Context, _ []Service, _ ServiceAction) {} // Default is no logs

func GenericActionLogger(_ context.Context, svcs []Service, action ServiceAction) {
	names := Services(svcs).GetNames()
	log.Printf("[CallServices]: Running stage %s for: %s\n", action, strings.Join(names, ","))
}

type Services []Service

func (s Services) GetNames() []string {
	names := []string{}
	for _, svc := range s {
		names = append(names, "\""+svc.Name()+"\"")
	}
	return names
}

// Recover calls and returns the internal r.Recover function if set, otherwise it returns an error
func (r *Recoverable) Recover(ctx context.Context) error {
	if r.Recovery != nil {
		return r.Recovery(ctx)
	}
	return errors.New("recovery not possible")
}

var _ Service = &DryRunService{}

func CallServices(ctx context.Context, services []Service, opts CallServicesOpts) ([]error, error) {
	errs := RunServiceAction(ctx, services, SERVICE_CHECK)
	if task.AnyError(errs) {
		if opts.OnActionError != nil {
			opts.OnActionError(ctx, SERVICE_CHECK, services, errs)
		}

		if ctx.Value("dryRun") == nil && ctx.Value("recover") != nil {
			if task.AnyError(RunServiceAction(ctx, services, SERVICE_RECOVER)) { // Recovery errors are discarded
				return errs, errors.New("unable to recover from one or more failed pre-run checks")
			}
		} else {
			return errs, errors.New("one or more pre-run checks failed")
		}
	}

	if ctx.Value("dryRun") == nil {
		errs = RunServiceAction(ctx, services, SERVICE_RUN)
		if task.AnyError(errs) {
			if opts.OnActionError != nil {
				opts.OnActionError(ctx, SERVICE_RUN, services, errs)
			}
			if !opts.SkipRollback {
				go RunServiceAction(ctx, services, SERVICE_ROLLBACK)
			}
			return errs, errors.New("one or more runs failed")
		}
	}
	return errs, nil
}

func CallServicesAndReply(ctx context.Context, services []Service, opts CallServicesOpts) (int, *Response) {
	errs, err := CallServices(ctx, services, opts)
	return GenerateResponse(services, errs, err)
}

func CallStagedServices(ctx context.Context, stages [][]Service, opts CallServicesOpts) (int, []error, error) {
	for i := 0; i < len(stages); i++ {
		if opts.OnStageStart != nil {
			opts.OnStageStart(ctx, stages[i])
		}
		errs, err := CallServices(ctx, stages[i], opts) // Note: opts.SkipRollback = true
		if err != nil {
			// Stage failed. Rollback all stages that ran in reversed order
			if !opts.SkipRollback {
				go func() {
					for j := i - 1; j >= 0; j-- { // We don't Roll back current stage because CallServices will do that
						RunServiceAction(ctx, stages[j], SERVICE_ROLLBACK)
					}
				}()
			}
			return i, errs, err
		}
	}

	return len(stages), nil, nil
}

func CallStagedServicesAndReply(ctx context.Context, stages [][]Service, opts CallServicesOpts) (int, *Response) {
	nStagesRun, errs, err := CallStagedServices(ctx, stages, opts)
	return GenerateStagedResponse(stages, nStagesRun, errs, err)
}

// ProtoService implements task.Runnable
var _ task.Runnable = &ProtoService{}

type ProtoService struct {
	service Service
	action  ServiceAction
}

type ServiceAction string

const (
	SERVICE_CHECK    ServiceAction = "CHECK"
	SERVICE_RECOVER  ServiceAction = "RECOVER"
	SERVICE_RUN      ServiceAction = "RUN"
	SERVICE_ROLLBACK ServiceAction = "ROLLBACK"
)

func (p ProtoService) Run(ctx context.Context) error {
	if p.action == SERVICE_CHECK {
		return p.service.Check(ctx)
	} else if p.action == SERVICE_RECOVER {
		return p.service.Recover(ctx)
	} else if p.action == SERVICE_RUN {
		return p.service.Run(ctx)
	} else if p.action == SERVICE_ROLLBACK {
		return p.service.Rollback(ctx)
	}
	return errors.New("ProtoService Run called with invalid action (did you init?): " + string(p.action))
}

func RunServiceAction(ctx context.Context, services []Service, action ServiceAction) []error {
	ActionLogger(ctx, services, action)

	// Convert []Service to []task.Runnable using ProtoService
	var tasks []task.Runnable
	for _, service := range services {
		tasks = append(tasks, ProtoService{service: service, action: action})
	}

	// Run all Services concurrently
	errs := task.Run(tasks, ctx)

	// In case of Rollback errors a reporter function is informed
	if action == SERVICE_ROLLBACK && RollbackErrorReporter != nil {
		go RollbackErrorReporter(ctx, services, errs)
	}
	return errs
}

func (d *DryRunService) Name() string {
	return d.Wrapper.Name() + " (dryRun)"
}

func (d *DryRunService) Check(ctx context.Context) error {
	if d.Wrapper == nil {
		return errors.New("DryRunService: no target service provided, nothing to dry-run")
	}
	return d.Wrapper.Check(ctx)
}

func (d *DryRunService) Run(_ context.Context) error {
	return nil // We are a dry run
}

func (d *DryRunService) Rollback(_ context.Context) error {
	return nil // We are a dry run
}

func (d *DryRunService) GetResponse(err error) any {
	return d.Wrapper.GetResponse(err)
}
