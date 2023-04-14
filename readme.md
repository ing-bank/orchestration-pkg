# ING Container Hosting Platform Orchestration Package

## About
The initial open-source release of this project is provided as-is. That implies that the codebase is only a slightly
modified version of what we at ING are using. In the future we would like to extend this Open Source repository with
the appropriate pipelines and contribution mechanics. This does imply that your journey for using this project for your
own purposes can use some improvement, and we will work on that.

This project is part of the ING Neoria. Neoria contains parts of the ING Container Hosting Platform (ICHP) stack
which is used to deliver Namespace-as-a-Service on top of OpenShift.

## Contents

* Overview
    * Service Response patterns
    * Service Request patterns
    * Recovery
    * Rollback
    * Dry Runs
    * Rest APIs to Services
    * (Multi-)Staged Service calls
* Example API
* Other

In short, the ICHP orchestration package runs many services at the same time, and joins the results. Every service
goes through a number of stages (Check, Run and optionally Rollback), and all services must have passed a stage before
moving to the next one (apart from Rollback, which is executed when any service fails its Run stage).

To summarize:

```text
// Define your Service
var _ Service = &MyService{}   // MyService implements Service
type MyService struct {
    orchestration.Recoverable, // Implements the Recover func to satisfy Service interface
    orchestration.Payload,     // Implements the GenerateResponse func to satisfy Service interface
    Datacenter string          // For example, to generate some complexity
}

func (svc *MyService) Name() string { return "MyService" }
func (svc *MyService) Check(_ context.Context) error { ... }
func (svc *MyService) Run(_ context.Context) error { ... }
func (svc *MyService) Rollback(_ context.Context) error { ... }

func main() {
    // Define the Services that you want to execute concurrently
    services := []Service{ &MyService{Datacenter: "DC1"}, &MyService{Datacenter: "DC2"}}
    
    // Call Check, Run, Rollback stages for each Service
    // errs align with services, err[i] corresponds with service[i]. err[i] may be nil
    // err is nil only when all errs are nil. Err contains information about which stage failed.
    errs, err := CallServices(context.TODO(), services)
    
    httpStatusCode, response := GenerateResponse(services, errs, err)
    // 200, {"status":"ok","details":[{"name":"MyService","detail":"<your-response>"}}
}
```

In the example above the services are executed in the following timeline, time flows right:

```text
-> | MyService{DC1}.Check | -> | MyService{DC1}.Run | -> Rollback Skipped
   | MyService{DC2}.Check |    | MyService{DC2}.Run |

```

## Overview

The `pkg/orchestration` package is designed for concurrent synchronous `Service` calls. The package goes through several
stages for each `Service`:

- **Check**: Sanity check whether the `Service` request is likely to succeed
- (**Recover**: Advanced usage to recover from a failing Check, used to recover from corrupted/illegal states)
- **Run**: Executes the `Service` request. All `Service`s must have passed their `Check` stage.
- (**Rollback**: Is called for every `Service` when one or more `Service` has failed their `Run` stage)

The `Service` interface can be found in `pkg/orchestration/api_service.go`, and looks as follows:

```text
type Service interface {
    Name() string
    
    Check(ctx context.Context) error
    Recover(ctx context.Context) error // Advanced usage, should return an error if not implemented
    Run(ctx context.Context) error
    Rollback(ctx context.Context) error
    
    GetResponse(err error) interface{}
}
```

Apart from the stages there is the `Name` method, for a human-readable representation of the `Service`. Finally, there
is the `GetResponse` method which should generate the output for the `Service`. Note that the `Service` itself has no
request or response payloads, these should be handled by your implementation.

When `Services` are defined, the `CallServices` function executes every `Service` concurrently, through the stages. That
means that for all `Service`s the `Check` stage will be called (concurrently). When all checks pass `CallServices` will
repeat that but with the `Run` method instead. For example:

```text
services := []Service{ &MyServiceA{Request: request}, &MyServiceB{Request: request} }
errs, err := CallServices(context.TODO(), services)

// 1. Calls MyServiceA.Check and MyServiceB.Check "at the same time"
// 2. When Checks pass, will call MyServiceA.Run and MyServiceB.Run at the same time
// 3. (if Any Run fails it will call MyServiceA.Rollback and MyServiceB.Rollback at the same time.

if err != nil {
    // Either the Check or the Run (or Recover: read more about this in advanced) staged failed
    // The index of Service is matched with errs, so Service[i] has errs[i], and errs[i] may be nil
}
```

An example of a `Service` implementation is given in `internal/example/create_my_service.go`. For more information
about `Recover`, checkout the *Recovery* section.

### Service Response patterns

Since the `Service` interface has no output, apart from the error, the output must be generated via your own
implementation. To help with this there is a `Payload` struct which has a response interface, and can be used to
generate a generic JSON response. E.g.

```text
struct MyService {
    Payload
}

func (svc *MyService) Run(_ context.Context) error {
    svc.Payload.Response = "Good!"
    return nil
}

func main() {
    services := []Services{ &MyService{} }
    errs, err := CallServices(context.TODO(), services)
    
    httpStatusCode, response := GenerateResponse(services, errs, err)
    
    // response: 200: {"status":"ok","details":[{"name":"MyService","detail":"Good!"}}
    // or, on check stage error: 500: {"status":"one or more pre-run checks failed","details":[{"name":"MyService","detail":"some-error"}}
    // or, on run stage error: 500: {"status":"one or more runs failed","details":[{"name":"MyService","detail":"some-error"}}
}
```

### Service Request patterns

A request payload should be contained in your `Service` implementation. It is advised to consider two types of payloads:

1. a `Service` local payload
2. a mutable payload shared across `Service`s (with an atomic lock)

In scenario 1 the payload is copied from the users request to your `Service` implementation. This allows the `Service`
to read and modify the payload freely without worrying about concurrency issues. In scenario 2 the payload is shared
across multiple `Service`s, this allows `Service`s to share state with each other. In doing so this variable is no
longer thread safe and has to be protected with an atomic lock, e.g. a Mutex. For example:

```text
type MyService struct {
    LocalRequest MyServiceRequest // Can be read/modified freely
    
    mutex.Lock // Owns SharedRequest
    SharedRequest *MyServiceRequest // Shared state across Services, protected by Lock.
}
```

### Recovery

In some scenarios it may be possible to recover from a failing `Check`. This can be useful in scenarios where a state
does not align with the ended state, e.g. due to a partial `Rollback`. Using `Recover` it is possible to rectify these
states. Recovery is only executed by `CallServices` when `recover` is set in the `context`. The recommended pattern for
recover operations is using a function pointer. Using the function pointer it is possible to set fine-grained behavior
depending on what error occurs during the Check stage, or perhaps `nil` if recovery is not possible.

```text
type MyUpdateService struct {
    Recoverable // Provides a MyUpdateService.Recovery function pointer, and satisfies the Recover method for Service
    Request Spec
}

func (svc *MyUpdateService) Check(_ context.Context) error {
    if !database.Has(svc.Request.Name) { // Example db and request
        svc.Recovery = func(ctx context.Context) error {
            return MyCreateService{Request: svc.Request}.Run(ctx)
        }
        return errors.new("cannot update " + svc.Request.Name + " because it is not found")
    }
    return nil
}
```

### Rollback

When a `Service` has a rollback it is executed "in the background", that means the `CallServices` has already returned
errors (cannot use CallServices errs to return rollback errors). Since a rollback is a fallible operation, generated
errors needs to be reported somewhere. This is what the `RollbackErrorReporter` is for. The orchestration package calls
the `RollbackErrorReporter` with all the services in a stage when a Service Rollback has an error.

To use it, assign a `func` to the variable `orchestration.RollbackErrorReporter`. In the example below all rollback
errors are printed to `stdout`.

```text

orchestration.RollbackErrorReporter = func(services []orchestration.Service, errs []error) {
    for i := 0; i < len(services); i++ {
        if errs[i] != nil {
            log.Printf("Rollback failed for Service %s: %v", services[i].Name(), errs[i])
        }
    }
}
```

### Dry Runs

When the dryRun flag is specified in the `Context` the `CallServices` function only executes the `Check` stage of
every `Service`. In essence, a dryRun will give a good indication of whether a request is likely to succeed.

```text
func main() {
    ctx := context.WithValue(context.Background(), "dryRun", true)
    _, _ = CallServices(ctx, []Services{ ... }) // Only calls Check stage for each Service
}
```

`recover` flags in `Context` are ignored when `dryRun` is specified.

### REST API to a Service interface

REST APIs can be generically converted to a `Service` interface. By relying on the REST contract `Checks`
and `Rollbacks` can be inferred. Note that while this mechanism can be handy, it removed the more fine-grained options
that a custom `Check` stage has. Converting a REST API automatically to a Service gives the following functionalities:

* **GET /api/v1/example/<name>** (Read example object with `name`):
    * Check: None
    * Run: Executes Get(), response is stored under svc.Response
    * Rollback: None


* **GET /api/v1/example** (List all objects in `example`):
    * Check: None
    * Run: Executes Get(), response is stored under svc.Response
    * Rollback: None

* **POST /api/v1/example** (Create object in request payload):
    * Check: Executes Get(), and expects an error
    * Run: Executes Post(), response is stored under svc.Response
    * Rollback: Executes Delete()


* **PUT /api/v1/example** (Update object in request payload):
    * Check: Executes Get(), stores it as `backup`
    * Run: Executes Put(), response is stored under svc.Response
    * Rollback: Executes Put() with payload stored under `backup`


* **DELETE /api/v1/example/<name>** (Delete example object with `name`):
    * Check: Executes Get(), stores it as `backup`
    * Run: Executes Delete(), response is stored under svc.Response
    * Rollback: Executes Create() with payload stored under `backup`

To achieve this functionality use the constructor function. E.g. to transform a REST API to a Creation `Service`:

```text
var request Nameable = &SomePostedPayload{}
name, err := request.Name() // TODO: handle err. Name is optional for POST/PUT

var exampleApi RestApi = &MyExampleApi{}
svc := RestApiAsService(exampleApi, "Example Create", name, request)

errs, err := CallServices(context.TODO(), []Service{svc})
```

This can be handy especially for dealing with external APIs that do not have dryRun functionalities. To compare it with
the example of the memory API in this repository, using the REST API conversion we can check for name conflicts and even
Rollbacks, but we cannot Check for available memory until the actual POST/PUT.

### Staged Service Calls

When a `Service` depends on the output of another `Service` it is not possible to executed them concurrently, there is a
dependency. To achieve this there is the option to run a list of `Service`s in stages. For example:

```text
sharedState := &Example{mutex.Lock{}, State: SomeState}
stageNum, errs, err := CallStagedServices(context.TODO(), [][]Service{
        {   // Stage 1
            &DependencyServiceA{SharedState: sharedState},
            &DependencyServiceB{SharedState: sharedState},
            MakeDryRun(&OtherServiceA{SharedState: sharedState}),
            MakeDryRun(&OtherServiceB{SharedState: sharedState}),
        },
        {   // Stage 2 - Executed after all Run's of stage 1 were successful
            &OtherServiceA{SharedState: sharedState}, // E.g. depends on DependencyServiceA
            &OtherServiceB{SharedState: sharedState}
        }
    }
}
```

In the example below the `CallStagedServices` function executes all services in stage 1 first. Only when they have all
completed the run stages successfully will it call stage 2. If there is an error in any service in stage 2, all Services
that have been called will be rolled back.

When running stage 1 it is often desireable to know whether services in stage 2 are likely to succeed. To execute only
the check stage of services they can be wrapped in a call to `MakeDryRun` which makes the run and rollback methods
stubs (`Name`, `Check` and `GetResponse` should be implemented).

# Example API

In this repository you can find two applications which both offer the Create Memory Claim service as an example. One
implementation implements the `Service` interface. Whereas the other implements the `RestAPI` interface, which can be
automatically transformed to a `Service`.

Advantages of implementing a `Service` over `RestApi`:

- Fine-grained control over the `Check` and `Rollback` stages
- Suitable for concurrent "expensive" `Run` calls

Disadvantages of `Service` over `RestApi`:

- More code, Check/Run/Rollback implementation for each operation (Create/Read/Update/Delete)
- No automatic implementation of `Check` and `Rollback` stages.

A `Service` requires custom code to implement Check/Run/Rollback stages for every operation. This is beneficial for the
runtime, but also increases code complexity and size. On the other hand the `RestApi` interface can be automatically
converted to a `Service`, which results in more compact code and automatic Checks and Rollbacks. However, these Checks
are less accurate than implementing your own, and thus may result in more failed Run stages.

The Memory Claim service example will create a memory claim in 4 Datacenter/Zones. In order to show some capabilities of
this package, it only holds one counter (starts at `2048`mb) for all datacenters. This will make it look like there is
enough memory available whilst in reality there only is 25% of that counter available. This allows us to see the Check
and Rollback stages that the package offers.

To do a successful create (due to POST) memory claim with size `100`mb (from `example-payloads`):

```text
$ curl http://localhost:8090/api/v1/memory -d @example-payloads/payload_ok.json
{"status":"ok","details":[
  {"name":"MyService Create DC1_BLUE","detail":"ok"},
  {"name":"MyService Create DC1_RED","detail":"ok"},
  {"name":"MyService Create DC2_BLUE","detail":"ok"},
  {"name":"MyService Create DC2_RED","detail":"ok"}
]}
```

After a successful Create claim, issuing another will conflict. This is checked during the Check stage:

```text
$ curl http://localhost:8090/api/v1/memory -d @example-payloads/payload_ok.json
{"status":"one or more pre-run checks failed","details":[
  {"name":"MyService Create DC1_BLUE","detail":"already exists"},
  {"name":"MyService Create DC1_RED","detail":"already exists"},
  {"name":"MyService Create DC2_BLUE","detail":"already exists"},
  {"name":"MyService Create DC2_RED","detail":"already exists"}
]}
```

If we restart the server, and try to create the claim with a too large size the Check stage will pass because of the
"wrong" counter handling. It will then execute the Run stage, but only succeed in making two claims, the rest will fail.
Because some claims have failed, it will Rollback, which means it will delete the two claims that were successful. After
this call no memory claim exists:

```text
$ curl http://localhost:8090/api/v1/memory -d @example-payloads/payload_fail.json
{"status":"one or more runs failed","details":[
  {"name":"MyService Create DC1_BLUE","detail":"ok"},
  {"name":"MyService Create DC1_RED","detail":"not enough memory available"},
  {"name":"MyService Create DC2_BLUE","detail":"not enough memory available"},
  {"name":"MyService Create DC2_RED","detail":"ok"}
]}
```

# Other (Just thoughts, basically):

We can use a Future to allow asynchronous requests in combination with REST compliant APIs.

E.g.

```text
func onConsumerPostRequest(request *Request) *Response {
    if request.State != nil && request.State.Future != nil {
        dependency := Get("dependency.service", request.Name, request.State.Future) # Get current state of dependency
        ...
    } else {
        future, err = Post("dependency.service", ToDependencyRequest(request)) # Async Post Action
        request.State.Future["dependency.service"] = future
        ...
    }
}
```

Typically, use the `Service` interface to offer services that you own and control. With this interface you can create
fine-grained pre-run Checks, Recovery, and Rollbacks. For external Services that do not offer a dry-run functionality
you can use the REST contract to create a recoverable Service. To achieve this, wrap the external endpoint to satisfy
the `RestApi` interface, and then call `RestApiAsService`.
