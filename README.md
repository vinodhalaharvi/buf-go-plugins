# Buf Go Plugins

Custom protoc plugins for generating Go backend code from Protocol Buffer definitions.

## Installation

```bash
# Install all plugins
go install github.com/vinodhalaharvi/buf-go-plugins/cmd/...@latest

# Or install specific plugins
go install github.com/vinodhalaharvi/buf-go-plugins/cmd/protoc-gen-firestore@latest
go install github.com/vinodhalaharvi/buf-go-plugins/cmd/protoc-gen-inmemory@latest
go install github.com/vinodhalaharvi/buf-go-plugins/cmd/protoc-gen-connect-server@latest
```

## Available Plugins

| Plugin | Output | Description |
|--------|--------|-------------|
| `protoc-gen-firestore` | `*_firestore.pb.go` | Firestore CRUD repository |
| `protoc-gen-inmemory` | `*_inmemory.pb.go` | In-memory repository (testing) |
| `protoc-gen-connect-server` | `*_connect_server.pb.go` | Connect HTTP server |
| `protoc-gen-validation` | `*_validation.pb.go` | Field validation |
| `protoc-gen-auth` | `*_auth.pb.go` | Authentication middleware |
| `protoc-gen-auth-email` | `*_auth_email.pb.go` | Email auth flow |
| `protoc-gen-auth-oauth` | `*_auth_oauth.pb.go` | OAuth2 flow |
| `protoc-gen-realtime` | `*_realtime.pb.go` | WebSocket/SSE subscriptions |
| `protoc-gen-graphql` | `*.graphql` | GraphQL schema |
| `protoc-gen-openapi` | `openapi.yaml` | OpenAPI spec |
| `protoc-gen-mock` | `*_mock.pb.go` | Mock implementations |
| `protoc-gen-test` | `*_test.go` | Test helpers |
| `protoc-gen-llm` | `tools.json` | LLM tool definitions |
| `protoc-gen-stripe` | `*_stripe.pb.go` | Stripe integration |
| `protoc-gen-notification` | `*_notification.pb.go` | Push notifications |
| `protoc-gen-geo` | `*_geo.pb.go` | Geospatial queries |
| `protoc-gen-react-admin` | React components | Admin UI |
| `protoc-gen-react-app` | React app | Full React app |
| `protoc-gen-wire` | `wire.go` | Dependency injection |
| `protoc-gen-deploy` | Dockerfile, k8s | Deployment manifests |

## Usage with Buf

### buf.gen.yaml

```yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/yourorg/yourproject/gen/go

plugins:
  # Standard Go protobuf
  - remote: buf.build/protocolbuffers/go
    out: gen/go
    opt: paths=source_relative

  # Connect RPC
  - remote: buf.build/connectrpc/go
    out: gen/go
    opt: paths=source_relative

  # === Custom Plugins ===

  # Firestore repository
  - local: protoc-gen-firestore
    out: gen/go
    opt: paths=source_relative

  # In-memory repository (for testing)
  - local: protoc-gen-inmemory
    out: gen/go
    opt: paths=source_relative

  # Connect server implementation
  - local: protoc-gen-connect-server
    out: gen/go
    opt: paths=source_relative

inputs:
  - directory: proto
```

### Generate

```bash
buf generate
```

### Output Structure

```
gen/go/
└── yourpackage/v1/
    ├── service.pb.go              # Protobuf messages
    ├── servicev1connect/
    │   └── service.connect.go     # Connect handlers interface
    ├── service_firestore.pb.go    # Firestore repository
    ├── service_inmemory.pb.go     # In-memory repository
    └── service_connect_server.pb.go # Server implementation
```

## Example

### Proto Definition

```protobuf
syntax = "proto3";
package example.v1;

message User {
  string user_id = 1;
  string email = 2;
  string name = 3;
}

service UserService {
  rpc CreateUser(CreateUserRequest) returns (User);
  rpc GetUser(GetUserRequest) returns (User);
  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse);
  rpc UpdateUser(UpdateUserRequest) returns (User);
  rpc DeleteUser(DeleteUserRequest) returns (DeleteUserResponse);
}
```

### Generated Firestore Repository

```go
// Auto-generated - DO NOT EDIT

type UserFirestoreRepository struct {
    client *firestore.Client
}

func NewUserFirestoreRepository(client *firestore.Client) *UserFirestoreRepository {
    return &UserFirestoreRepository{client: client}
}

func (r *UserFirestoreRepository) Create(ctx context.Context, user *User) error { ... }
func (r *UserFirestoreRepository) Get(ctx context.Context, id string) (*User, error) { ... }
func (r *UserFirestoreRepository) List(ctx context.Context, opts ...ListOption) ([]*User, error) { ... }
func (r *UserFirestoreRepository) Update(ctx context.Context, user *User) error { ... }
func (r *UserFirestoreRepository) Delete(ctx context.Context, id string) error { ... }
```

### Generated In-Memory Repository

```go
// Auto-generated - DO NOT EDIT

type UserInMemoryRepository struct {
    mu    sync.RWMutex
    users map[string]*User
}

func NewUserInMemoryRepository() *UserInMemoryRepository {
    return &UserInMemoryRepository{users: make(map[string]*User)}
}

// Same interface as Firestore - swap at runtime!
func (r *UserInMemoryRepository) Create(ctx context.Context, user *User) error { ... }
func (r *UserInMemoryRepository) Get(ctx context.Context, id string) (*User, error) { ... }
```

### Usage in main.go

```go
package main

import (
    "context"
    "net/http"

    "cloud.google.com/go/firestore"
    examplev1 "github.com/yourorg/yourproject/gen/go/example/v1"
)

func main() {
    ctx := context.Background()
    
    // Production: Firestore
    client, _ := firestore.NewClient(ctx, "your-project")
    userRepo := examplev1.NewUserFirestoreRepository(client)
    
    // Testing: In-memory (same interface!)
    // userRepo := examplev1.NewUserInMemoryRepository()
    
    // Create server with repository
    server := examplev1.NewUserServiceServer(userRepo)
    
    // Register Connect handlers
    mux := http.NewServeMux()
    mux.Handle(examplev1connect.NewUserServiceHandler(server))
    
    http.ListenAndServe(":8080", mux)
}
```

## Swapping Backends

All repositories implement the same interface, so you can swap at runtime:

```go
type UserRepository interface {
    Create(ctx context.Context, user *User) error
    Get(ctx context.Context, id string) (*User, error)
    List(ctx context.Context, opts ...ListOption) ([]*User, error)
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
}

// Both implement UserRepository
var _ UserRepository = (*UserFirestoreRepository)(nil)
var _ UserRepository = (*UserInMemoryRepository)(nil)
```

## Plugin Options

### protoc-gen-firestore

```yaml
- local: protoc-gen-firestore
  out: gen/go
  opt:
    - paths=source_relative
    - soft_delete=true      # Add deleted_at field
    - timestamps=true       # Add created_at, updated_at
```

### protoc-gen-connect-server

```yaml
- local: protoc-gen-connect-server
  out: gen/go
  opt:
    - paths=source_relative
    - cors=true            # Add CORS middleware
    - auth=true            # Add auth middleware
```

## License

MIT

