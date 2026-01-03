# protoc-gen-category

A Buf/protoc plugin that generates formal category theory types for Go from Protocol Buffer definitions.

## Features

### Type-Level Abstractions
- **Morphism** - Functions between types with identity and composition
- **Semigroup** - Associative binary operation (Combine)
- **Monoid** - Semigroup with identity element (Empty + Combine)
- **Functor** - FMap for slices of message types
- **Foldable** - FoldLeft, FoldRight, FoldMap
- **Bifunctor** - Bimap, MapLeft, MapRight for Result types

### Effect Types
Three concrete effect types representing IO operations:
- **NetworkOp[A]** - Network/HTTP operations
- **DBOp[A]** - Database operations
- **DiskOp[A]** - Filesystem operations

Each effect type includes:
- **Pure** - Lift pure value into effect
- **FMap** - Apply function inside effect (Functor)
- **Ap** - Apply wrapped function to wrapped value (Applicative)
- **Lift2** - Lift binary function into effect
- **Bind** - Chain effectful computations (Monad)
- **ComposeKleisli** - Compose Kleisli arrows
- **Traverse** - Apply effectful function to slice elements
- **TraverseParallel** - Parallel traverse using errgroup
- **Sequence** - Convert []Effect[A] to Effect[[]A]
- **Retry** - Retry with exponential backoff
- **Fallback** - Try primary, fallback on error
- **Timeout** - Add timeout to effect

### Natural Transformations
Automatic lifting between effect types:
- `LiftNetworkOpToDBOp`
- `LiftNetworkOpToDiskOp`
- `LiftDBOpToNetworkOp`
- etc.

### Service Abstractions
- **Kleisli Service** - Service as product of Kleisli arrows
- **Typed Middleware** - Per-method middleware with full type safety
- **Parallel Execution** - Parallel request handling
- **Fanout** - Call multiple services, combine with monoid
- **Circuit Breaker** - Failure protection
- **Mock Generation** - Mock implementations for testing

## Installation

```bash
go install github.com/example/protoc-gen-category/cmd/protoc-gen-category@latest
```

## Usage

### Proto Schema

First, import the category options:

```protobuf
syntax = "proto3";

import "category/options.proto";

// Enable effect types at file level
option (category.category_file) = {
    effects: [NETWORK, DATABASE, DISK]
};
```

### Message Options

Add category theory support to messages:

```protobuf
message User {
    option (category.category) = {
        functor: true           // FMap for []User
        monoid: true            // Empty + Combine
        foldable: true          // Fold operations
        traversable: true       // Traverse for effects
        monad: true             // Bind for effects
        firestore_bridge: true  // Firestore CRUD, queries, transactions
    };

    string id = 1;
    string name = 2;
    int64 score = 3 [(category.field_category) = { combine: SUM }];
}
```

### Combine Strategies

Control how fields are combined in Semigroup/Monoid:

| Strategy | Description | Default for |
|----------|-------------|-------------|
| INFER | Auto-detect from type | - |
| CONCAT | Concatenate | string, bytes, repeated |
| SUM | Add | numeric types |
| PRODUCT | Multiply | - |
| ALL | Logical AND | bool |
| ANY | Logical OR | - |
| FIRST | First non-zero | - |
| LAST | Last non-zero | - |
| MERGE | Deep merge | messages |

### Service Options

```protobuf
service UserService {
    option (category.category_service) = {
        kleisli: true         // Kleisli arrow composition
        middleware: true      // Typed middleware
        parallel: true        // Parallel execution
        retry: true           // Retry support
        circuit_breaker: true // Circuit breaker
        fanout: true          // Fanout with monoid
        mock: true            // Mock generation
        connect_bridge: true  // Connect-go adapters
    };

    rpc GetUser(GetUserRequest) returns (User) {
        option (category.category_method) = {
            idempotent: true
        };
    }
}
```

## Connect-go Integration

When `connect_bridge: true` is set, the plugin generates adapters to integrate with [Connect-go](https://connectrpc.com/):

### Generated Types

```go
// Interface matching Connect-generated client
type UserServiceConnectClient interface {
    GetUser(context.Context, *connect.Request[GetUserRequest]) (*connect.Response[User], error)
    // ...
}

// Wrap Connect client as Kleisli arrows
func NewUserServiceKFromConnect(client UserServiceConnectClient) *UserServiceK

// Wrap with default resilience (timeout + retry for idempotent methods)
func NewUserServiceKFromConnectWithDefaults(
    client UserServiceConnectClient,
    timeout time.Duration,
    maxRetries int,
) *UserServiceK

// Connect handler backed by Kleisli service
type UserServiceConnectHandler struct { ... }
func NewUserServiceConnectHandler(svc *UserServiceK) *UserServiceConnectHandler
```

### Usage: Resilient Connect Client

```go
// Create Connect client
connectClient := userv1connect.NewUserServiceClient(
    http.DefaultClient,
    "https://api.example.com",
)

// Wrap with CT abstractions + default resilience
userService := NewUserServiceKFromConnectWithDefaults(
    connectClient,
    5*time.Second,  // timeout
    3,              // max retries (only for idempotent methods)
)

// Now use Kleisli composition, parallel execution, etc.
users, err := userService.ParallelGetUser(requests)(ctx)
```

### Usage: Connect Server with CT

```go
// Build Kleisli service with business logic
svc := &UserServiceK{
    GetUser: func(req *GetUserRequest) NetworkOp[*User] {
        return BindNetwork(
            validateRequest(req),
            func(_ bool) NetworkOp[*User] {
                return fetchFromDB(req.Id)
            },
        )
    },
    // ...
}

// Add middleware
withLogging := ApplyUserServiceMiddleware(svc, loggingMiddleware)

// Create Connect handler
handler := NewUserServiceConnectHandler(withLogging)

// Mount on Connect
mux := http.NewServeMux()
path, h := userv1connect.NewUserServiceHandler(handler)
mux.Handle(path, h)

http.ListenAndServe(":8080", mux)
```

### Multi-Region Fanout

```go
// Connect clients to multiple regions
usEast := NewUserServiceKFromConnect(usEastClient)
usWest := NewUserServiceKFromConnect(usWestClient)
euWest := NewUserServiceKFromConnect(euWestClient)

// Fanout reads, combine with monoid
globalService := &UserServiceK{
    GetUser: FanoutGetUser(
        []*UserServiceK{usEast, usWest, euWest},
        UserMonoid,
    ),
}
```

## Firestore Integration

When `firestore_bridge: true` is set on a message, the plugin generates Firestore collection, document, query, and transaction operations.

### Proto Setup

```protobuf
message User {
    option (category.category) = {
        monoid: true
        firestore_bridge: true
    };
    
    string id = 1;
    string name = 2;
    string email = 3;
    int64 score = 4;
}
```

### Generated Types

```go
// Collection accessor
type UserCollection struct { ... }
func NewUserCollection(client *firestore.Client) *UserCollection
func NewUserCollectionWithPath(client *firestore.Client, path string) *UserCollection

// Document operations (all return DBOp)
type UserDoc struct { ... }
func (d *UserDoc) Get() DBOp[*User]
func (d *UserDoc) Set(data *User) DBOp[bool]
func (d *UserDoc) SetMerge(data *User) DBOp[bool]
func (d *UserDoc) Create(data *User) DBOp[bool]
func (d *UserDoc) Update(updates []firestore.Update) DBOp[bool]
func (d *UserDoc) Delete() DBOp[bool]
func (d *UserDoc) Exists() DBOp[bool]

// Query builder
type UserQuery struct { ... }
func (c *UserCollection) Where(path, op string, value interface{}) *UserQuery
func (q *UserQuery) Where(path, op string, value interface{}) *UserQuery
func (q *UserQuery) OrderBy(path string, dir firestore.Direction) *UserQuery
func (q *UserQuery) Limit(n int) *UserQuery
func (q *UserQuery) GetAll() DBOp[[]*User]
func (q *UserQuery) First() DBOp[*User]
func (q *UserQuery) Count() DBOp[int64]

// Transaction monad
type UserTx[A any] func(ctx context.Context, tx *firestore.Transaction) (A, error)
func PureUserTx[A any](a A) UserTx[A]
func BindUserTx[A, B any](fa UserTx[A], f func(A) UserTx[B]) UserTx[B]
func (d *UserDoc) GetTx() UserTx[*User]
func (d *UserDoc) SetTx(data *User) UserTx[bool]
func (c *UserCollection) RunTransaction(op UserTx[*User]) DBOp[*User]

// Batch operations
func (c *UserCollection) BatchSet(items map[string]*User) DBOp[bool]
func (c *UserCollection) BatchDelete(ids []string) DBOp[bool]
func (c *UserCollection) GetMultiple(ids []string) DBOp[[]*User]
func (c *UserCollection) GetMultipleParallel(ids []string) DBOp[[]*User]
```

### Usage: Basic CRUD

```go
users := NewUserCollection(firestoreClient)

// Create
_, err := users.Doc("user-123").Set(&User{
    Id:    "user-123",
    Name:  "Alice",
    Email: "alice@example.com",
})(ctx)

// Read
user, err := users.Doc("user-123").Get()(ctx)

// Update
_, err = users.Doc("user-123").Update([]firestore.Update{
    {Path: "score", Value: firestore.Increment(10)},
})(ctx)

// Delete
_, err = users.Doc("user-123").Delete()(ctx)
```

### Usage: Queries

```go
// Find active users with score > 100
activeUsers, err := users.
    Where("status", "==", "active").
    Where("score", ">", 100).
    OrderBy("score", firestore.Desc).
    Limit(10).
    GetAll()(ctx)

// Get first match
topUser, err := users.
    Where("status", "==", "active").
    OrderBy("score", firestore.Desc).
    First()(ctx)

// Count
count, err := users.Where("status", "==", "active").Count()(ctx)
```

### Usage: Transactions

```go
// Transfer credits between users (atomic)
transfer := BindUserTx(
    users.Doc("alice").GetTx(),
    func(alice *User) UserTx[bool] {
        return BindUserTx(
            users.Doc("bob").GetTx(),
            func(bob *User) UserTx[bool] {
                alice.Score -= 100
                bob.Score += 100
                return BindUserTx(
                    users.Doc("alice").SetTx(alice),
                    func(_ bool) UserTx[bool] {
                        return users.Doc("bob").SetTx(bob)
                    },
                )
            },
        )
    },
)

success, err := users.RunTransaction(transfer)(ctx)
```

### Usage: Batch Operations

```go
// Batch set
newUsers := map[string]*User{
    "user-1": {Id: "user-1", Name: "Alice"},
    "user-2": {Id: "user-2", Name: "Bob"},
    "user-3": {Id: "user-3", Name: "Charlie"},
}
_, err := users.BatchSet(newUsers)(ctx)

// Batch delete
_, err = users.BatchDelete([]string{"user-1", "user-2", "user-3"})(ctx)

// Parallel fetch
ids := []string{"user-1", "user-2", "user-3"}
allUsers, err := users.GetMultipleParallel(ids)(ctx)
```

### Usage: With Resilience

```go
// Retry on transient errors
user, err := RetryDB(
    users.Doc("user-123").Get(),
    3, 100*time.Millisecond, 2*time.Second,
)(ctx)

// Combine with Connect: fetch from API, save to Firestore
syncUser := BindNetwork(
    userService.GetUser(&GetUserRequest{Id: id}),
    func(user *User) NetworkOp[bool] {
        return LiftDBOpToNetworkOp(users.Doc(user.Id).Set(user))
    },
)
```

## Multi-Tenancy

The Firestore bridge supports automatic tenant scoping via context. No proto annotations needed for most cases.

### How It Works

```go
// Generated context helpers
func WithTenantID(ctx context.Context, tenantID string) context.Context
func TenantIDFromContext(ctx context.Context) (string, bool)
func MustTenantID(ctx context.Context) string
```

When tenant ID is in context, all Firestore operations automatically scope to:
```
tenants/{tenant_id}/{collection}/{doc_id}
```

When no tenant ID is present, operations use the default path:
```
{collection}/{doc_id}
```

### Usage with JWT Middleware

Your existing JWT middleware just needs one line:

```go
func JWTMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims := validateJWT(r)
        
        // Add tenant to context - generated helper
        ctx := userpb.WithTenantID(r.Context(), claims.TenantID)
        
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

Now all downstream Firestore operations are automatically tenant-scoped:

```go
// These automatically go to tenants/{tenant_id}/users/...
users := NewUserCollection(client)
user, err := users.Doc("user-123").Get()(ctx)  // tenant-scoped!
users, err := users.Where("status", "==", "active").GetAll()(ctx)  // tenant-scoped!
```

### Global Collections

For cross-tenant data (system config, audit logs), use `global: true`:

```protobuf
message SystemConfig {
    option (category.category) = {
        firestore_bridge: true
        global: true  // Never tenant-scoped
    };
    
    string id = 1;
    string version = 2;
}
```

```go
// Always goes to system_configs/... regardless of tenant in context
config := NewSystemConfigCollection(client)
cfg, err := config.Doc("main").Get()(ctx)  // NOT tenant-scoped
```

### Path Resolution Summary

| Tenant in Context | `global: true` | Path |
|-------------------|----------------|------|
| No | No | `{collection}/{id}` |
| Yes | No | `tenants/{tenant}/{collection}/{id}` |
| No | Yes | `{collection}/{id}` |
| Yes | Yes | `{collection}/{id}` |

## Generated Code Examples

### Monoid Usage

```go
// Combine users
combined := UserMonoid.Combine(user1, user2)

// Fold slice of users
allUsers := FoldMap(users, Id[*User](), UserMonoid)
```

### Effect Chaining

```go
// Chain network calls
getAndProcess := BindNetwork(
    fetchUser(userID),
    func(u *User) NetworkOp[*Order] {
        return fetchOrders(u.Id)
    },
)

// Run with context
order, err := getAndProcess(ctx)
```

### Kleisli Composition

```go
// Compose service calls
getUserOrders := ComposeKleisliNetwork(
    userService.GetUser,
    func(u *User) NetworkOp[*ListOrdersResponse] {
        return orderService.ListOrders(&ListOrdersRequest{UserId: u.Id})
    },
)
```

### Parallel Execution

```go
// Fetch multiple users in parallel
users, err := svc.ParallelGetUser(requests)(ctx)
```

### Middleware

```go
mw := &UserServiceMiddleware{
    GetUser: func(next func(*GetUserRequest) NetworkOp[*User]) func(*GetUserRequest) NetworkOp[*User] {
        return func(req *GetUserRequest) NetworkOp[*User] {
            log.Printf("GetUser called: %s", req.Id)
            return next(req)
        }
    },
}
wrapped := ApplyUserServiceMiddleware(svc, mw)
```

### Circuit Breaker

```go
cb := NewUserServiceCircuitBreaker(svc, 5, time.Minute)
user, err := cb.GetUser(req)(ctx) // Protected by circuit breaker
```

### Testing with Mocks

```go
mock := NewMockUserServiceK()
mock.GetUserFunc = func(req *GetUserRequest) NetworkOp[*User] {
    return PureNetwork(&User{Id: req.Id, Name: "Test"})
}

// Use mock.ToKleisli() where *UserServiceK is expected
```

## Stripe Integration

When Stripe is configured at the file level, the plugin generates billing infrastructure.

### Proto Setup

```protobuf
option (category.category_file) = {
    effects: [NETWORK, DATABASE]
    stripe: {
        plans: ["free", "pro", "enterprise"]
        webhook_secret_env: "STRIPE_WEBHOOK_SECRET"
        api_key_env: "STRIPE_SECRET_KEY"
    }
};

message User {
    option (category.category) = {
        firestore_bridge: true
        stripe_customer: true  // Generate customer operations
    };
    
    string id = 1;
    string email = 2;
    string stripe_customer_id = 3;  // Auto-detected
}

message Subscription {
    option (category.category) = {
        firestore_bridge: true
        stripe_subscription: true  // Generate subscription operations
    };
    
    string id = 1;
    string user_id = 2;
    string stripe_subscription_id = 3;
    string plan = 4;
}

service ProjectService {
    option (category.category_service) = {
        kleisli: true
        stripe_billing: true  // Generate billing interceptor
    };
    
    rpc GetProject(GetProjectRequest) returns (Project) {
        option (category.category_method) = {
            min_plan: "free"
        };
    }
    
    rpc ExportProject(ExportRequest) returns (ExportResponse) {
        option (category.category_method) = {
            min_plan: "pro"
        };
    }
    
    rpc BulkExport(BulkExportRequest) returns (BulkExportResponse) {
        option (category.category_method) = {
            min_plan: "enterprise"
            metered: true
            meter_event: "bulk_exports"
        };
    }
}
```

### Generated Code

```go
// Plan type and constants
type Plan string
const (
    PlanFree       Plan = "free"
    PlanPro        Plan = "pro"
    PlanEnterprise Plan = "enterprise"
)

// Customer operations
type UserStripeOps struct { ... }
func NewUserStripeOps(collection *UserCollection) *UserStripeOps
func (s *UserStripeOps) CreateCustomer(id string) NetworkOp[*User]
func (s *UserStripeOps) GetOrCreateCustomer(id string) NetworkOp[*User]

// Subscription operations
type SubscriptionStripeSubOps struct { ... }
func NewSubscriptionStripeSubOps(collection *SubscriptionCollection, priceIDs map[Plan]string) *SubscriptionStripeSubOps
func (s *SubscriptionStripeSubOps) Subscribe(customerID string, plan Plan, paymentMethodID string) NetworkOp[*Subscription]
func (s *SubscriptionStripeSubOps) Cancel(subscriptionID string) NetworkOp[*Subscription]

// Webhook handler
func NewSubscriptionWebhookHandler(collection *SubscriptionCollection, webhookSecret string) http.Handler

// Billing interceptor
func ProjectServiceBillingInterceptor(getUserPlan func(ctx context.Context, userID string) (Plan, error)) connect.UnaryInterceptorFunc
```

### Usage

```go
func main() {
    users := NewUserCollection(firestoreClient)
    subscriptions := NewSubscriptionCollection(firestoreClient)
    
    // Customer operations
    userStripe := NewUserStripeOps(users)
    user, err := userStripe.GetOrCreateCustomer("user-123")(ctx)
    
    // Subscription operations
    priceIDs := map[Plan]string{
        PlanFree:       "price_free",
        PlanPro:        "price_pro",
        PlanEnterprise: "price_enterprise",
    }
    subStripe := NewSubscriptionStripeSubOps(subscriptions, priceIDs)
    sub, err := subStripe.Subscribe(user.StripeCustomerId, PlanPro, "pm_xxx")(ctx)
    
    // Service with billing
    getUserPlan := func(ctx context.Context, userID string) (Plan, error) {
        sub, err := subscriptions.Where("user_id", "==", userID).First()(ctx)
        if err != nil || sub == nil {
            return PlanFree, nil
        }
        return Plan(sub.Plan), nil
    }
    
    projectService := NewProjectServiceHandler(...)
    
    mux := http.NewServeMux()
    path, handler := projectv1connect.NewProjectServiceHandler(
        projectService,
        connect.WithInterceptors(
            ProjectServiceBillingInterceptor(getUserPlan),
        ),
    )
    mux.Handle(path, handler)
    
    // Webhook
    mux.Handle("/stripe/webhook", NewSubscriptionWebhookHandler(
        subscriptions,
        os.Getenv("STRIPE_WEBHOOK_SECRET"),
    ))
}
```

## Buf Integration

buf.gen.yaml:

```yaml
version: v1
plugins:
  - plugin: go
    out: gen/go
    opt: paths=source_relative
  - plugin: category
    out: gen/go
    opt: paths=source_relative
```

Then run:

```bash
buf generate
```

## Directory Structure

```
protoc-gen-category/
├── cmd/protoc-gen-category/
│   └── main.go           # Plugin entry point
├── internal/generator/
│   ├── generator.go      # Core generation logic
│   └── options_types.go  # Proto extension parsing
├── proto/category/
│   └── options.proto     # Category option definitions
├── example/
│   └── example.proto     # Example usage
├── go.mod
├── buf.yaml
└── README.md
```

## Dependencies

- Go 1.21+
- google.golang.org/protobuf
- golang.org/x/sync/errgroup

## License

MIT
