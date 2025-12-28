// protoc-gen-react-app generates a complete React application
// Includes: App shell, navigation, all CRUD pages, forms, tables, auth
// Beautiful modern UI with Tailwind CSS
package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
)

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		for _, f := range gen.Files {
			if !f.Generate || len(f.Messages) == 0 {
				continue
			}
			generateApp(gen, f)
		}
		return nil
	})
}

// Feature detection
type Features struct {
	HasAuthEmail    bool
	HasAuthOAuth    bool
	HasStripe       bool
	HasNotification bool
	HasGeo          bool
	HasRealtime     bool
	UserEntity      string // Name of entity with auth/stripe/notification embedded
}

func detectFeatures(file *protogen.File) Features {
	f := Features{}
	for _, msg := range file.Messages {
		name := msg.GoIdent.GoName
		switch name {
		case "AuthEmail":
			f.HasAuthEmail = true
		case "AuthOAuth":
			f.HasAuthOAuth = true
		case "StripeCustomer":
			f.HasStripe = true
		case "NotificationPrefs":
			f.HasNotification = true
		}
		// Check for geo fields
		for _, field := range msg.Fields {
			fn := string(field.Desc.Name())
			if fn == "latitude" || fn == "lat" {
				f.HasGeo = true
			}
		}
		// Find user entity (has auth embedded)
		for _, field := range msg.Fields {
			if field.Message != nil {
				mn := field.Message.GoIdent.GoName
				if mn == "AuthEmail" || mn == "AuthOAuth" || mn == "StripeCustomer" {
					f.UserEntity = name
				}
			}
		}
	}
	return f
}

func generateApp(gen *protogen.Plugin, file *protogen.File) {
	basePath := strings.Replace(file.GeneratedFilenamePrefix, "/go/", "/ui/", 1)
	basePath = strings.TrimSuffix(basePath, "/models")
	basePath = strings.TrimSuffix(basePath, "/v1")

	// Detect features
	features := detectFeatures(file)

	// Collect all entities and group by prefix for nav
	entities := []Entity{}
	for _, msg := range file.Messages {
		// Skip embedded types
		name := msg.GoIdent.GoName
		if isEmbeddedType(name) {
			continue
		}
		entities = append(entities, Entity{
			Name:   name,
			Fields: collectFields(msg),
		})
	}

	// Generate all files
	genFile(gen, basePath+"/src/App.tsx", generateAppTsx(entities, features))
	genFile(gen, basePath+"/src/main.tsx", generateMainTsx())
	genFile(gen, basePath+"/src/index.css", generateIndexCss())
	genFile(gen, basePath+"/src/types.ts", generateTypes(entities))
	genFile(gen, basePath+"/src/api.ts", generateApi(entities, features))
	genFile(gen, basePath+"/src/hooks.ts", generateHooks(entities))

	// Layout components
	genFile(gen, basePath+"/src/components/Layout.tsx", generateLayout(entities))
	genFile(gen, basePath+"/src/components/Sidebar.tsx", generateSidebar(entities, features))
	genFile(gen, basePath+"/src/components/Topbar.tsx", generateTopbar(features))
	genFile(gen, basePath+"/src/components/Breadcrumb.tsx", generateBreadcrumb())

	// Shared components
	genFile(gen, basePath+"/src/components/ui/Button.tsx", generateButton())
	genFile(gen, basePath+"/src/components/ui/Input.tsx", generateInput())
	genFile(gen, basePath+"/src/components/ui/Table.tsx", generateTable())
	genFile(gen, basePath+"/src/components/ui/Modal.tsx", generateModal())
	genFile(gen, basePath+"/src/components/ui/Card.tsx", generateCard())
	genFile(gen, basePath+"/src/components/ui/Badge.tsx", generateBadge())
	genFile(gen, basePath+"/src/components/ui/Dropdown.tsx", generateDropdown())
	genFile(gen, basePath+"/src/components/ui/Toast.tsx", generateToast())
	genFile(gen, basePath+"/src/components/ui/Loading.tsx", generateLoading())
	genFile(gen, basePath+"/src/components/ui/EmptyState.tsx", generateEmptyState())
	genFile(gen, basePath+"/src/components/ui/Pagination.tsx", generatePagination())
	genFile(gen, basePath+"/src/components/ui/SearchInput.tsx", generateSearchInput())
	genFile(gen, basePath+"/src/components/ui/index.ts", generateUiIndex())

	// Pages for each entity
	for _, e := range entities {
		genFile(gen, basePath+"/src/pages/"+e.Name+"ListPage.tsx", generateListPage(e))
		genFile(gen, basePath+"/src/pages/"+e.Name+"DetailPage.tsx", generateDetailPage(e))
		genFile(gen, basePath+"/src/pages/"+e.Name+"CreatePage.tsx", generateCreatePage(e))
		genFile(gen, basePath+"/src/pages/"+e.Name+"EditPage.tsx", generateEditPage(e))
		genFile(gen, basePath+"/src/components/"+e.Name+"Form.tsx", generateForm(e))
		genFile(gen, basePath+"/src/components/"+e.Name+"Table.tsx", generateEntityTable(e))
	}

	// Dashboard
	genFile(gen, basePath+"/src/pages/DashboardPage.tsx", generateDashboard(entities))

	// Auth pages (always generate basic login/signup)
	genFile(gen, basePath+"/src/pages/LoginPage.tsx", generateLoginPage(features))
	genFile(gen, basePath+"/src/pages/SignupPage.tsx", generateSignupPage(features))
	genFile(gen, basePath+"/src/context/AuthContext.tsx", generateAuthContext())

	// Feature-specific pages
	if features.HasAuthEmail {
		genFile(gen, basePath+"/src/pages/ForgotPasswordPage.tsx", generateForgotPasswordPage())
		genFile(gen, basePath+"/src/pages/ResetPasswordPage.tsx", generateResetPasswordPage())
		genFile(gen, basePath+"/src/pages/VerifyEmailPage.tsx", generateVerifyEmailPage())
	}

	if features.HasAuthOAuth {
		genFile(gen, basePath+"/src/pages/OAuthCallbackPage.tsx", generateOAuthCallbackPage())
		genFile(gen, basePath+"/src/pages/LinkedAccountsPage.tsx", generateLinkedAccountsPage())
	}

	if features.HasStripe {
		genFile(gen, basePath+"/src/pages/PricingPage.tsx", generatePricingPage())
		genFile(gen, basePath+"/src/pages/BillingPage.tsx", generateBillingPage())
		genFile(gen, basePath+"/src/pages/CheckoutSuccessPage.tsx", generateCheckoutSuccessPage())
	}

	if features.HasNotification {
		genFile(gen, basePath+"/src/pages/NotificationsPage.tsx", generateNotificationsPage())
		genFile(gen, basePath+"/src/pages/NotificationSettingsPage.tsx", generateNotificationSettingsPage())
		genFile(gen, basePath+"/src/components/NotificationBell.tsx", generateNotificationBellComponent())
	}

	if features.HasGeo {
		genFile(gen, basePath+"/src/pages/StoreLocatorPage.tsx", generateStoreLocatorPage())
		genFile(gen, basePath+"/src/components/MapView.tsx", generateMapViewComponent())
	}

	// Settings page (profile, security, etc.)
	genFile(gen, basePath+"/src/pages/SettingsPage.tsx", generateSettingsPage(features))
	genFile(gen, basePath+"/src/pages/ProfilePage.tsx", generateProfilePage())

	// Config files
	genFile(gen, basePath+"/package.json", generatePackageJson(features))
	genFile(gen, basePath+"/vite.config.ts", generateViteConfig())
	genFile(gen, basePath+"/tailwind.config.js", generateTailwindConfig())
	genFile(gen, basePath+"/postcss.config.js", generatePostcssConfig())
	genFile(gen, basePath+"/tsconfig.json", generateTsConfig())
	genFile(gen, basePath+"/index.html", generateIndexHtml())
}

func genFile(gen *protogen.Plugin, path, content string) {
	f := gen.NewGeneratedFile(path, "")
	f.P(content)
}

func isEmbeddedType(name string) bool {
	embedded := []string{"AuthEmail", "AuthOAuth", "OAuthLink", "StripeCustomer", "NotificationPrefs", "GeoLocation"}
	for _, e := range embedded {
		if name == e {
			return true
		}
	}
	return false
}

type Entity struct {
	Name   string
	Fields []Field
}

type Field struct {
	Name       string
	GoName     string
	Type       string
	TsType     string
	InputType  string
	IsRequired bool
	IsId       bool
	IsList     bool
	IsMessage  bool
	EnumValues []string
}

func collectFields(msg *protogen.Message) []Field {
	fields := []Field{}
	for _, f := range msg.Fields {
		field := Field{
			Name:       string(f.Desc.Name()),
			GoName:     f.GoName,
			IsRequired: f.Desc.Name() == "id",
			IsId:       string(f.Desc.Name()) == "id",
			IsList:     f.Desc.IsList(),
		}

		if f.Message != nil {
			field.IsMessage = true
			field.Type = f.Message.GoIdent.GoName
			if strings.Contains(field.Type, "Timestamp") {
				field.TsType = "string"
				field.InputType = "datetime-local"
			} else {
				field.TsType = field.Type
				field.InputType = "text"
			}
		} else if f.Enum != nil {
			field.Type = "enum"
			field.TsType = "string"
			field.InputType = "select"
			for _, v := range f.Enum.Values {
				field.EnumValues = append(field.EnumValues, string(v.Desc.Name()))
			}
		} else {
			field.Type, field.TsType, field.InputType = protoToTypes(f.Desc.Kind())
		}

		if field.IsList {
			field.TsType = field.TsType + "[]"
		}

		fields = append(fields, field)
	}
	return fields
}

func protoToTypes(kind protoreflect.Kind) (string, string, string) {
	switch kind {
	case protoreflect.BoolKind:
		return "bool", "boolean", "checkbox"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "int32", "number", "number"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "int64", "number", "number"
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return "float", "number", "number"
	case protoreflect.StringKind:
		return "string", "string", "text"
	case protoreflect.BytesKind:
		return "bytes", "string", "text"
	default:
		return "string", "string", "text"
	}
}

// =============================================================================
// MAIN APP FILES
// =============================================================================

func generateAppTsx(entities []Entity, features Features) string {
	routes := ""
	for _, e := range entities {
		lower := strings.ToLower(e.Name)
		routes += fmt.Sprintf(`        <Route path="/%s" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><%sListPage /></Suspense></Layout></ProtectedRoute>} />
        <Route path="/%s/new" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><%sCreatePage /></Suspense></Layout></ProtectedRoute>} />
        <Route path="/%s/:id" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><%sDetailPage /></Suspense></Layout></ProtectedRoute>} />
        <Route path="/%s/:id/edit" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><%sEditPage /></Suspense></Layout></ProtectedRoute>} />
`, lower, e.Name, lower, e.Name, lower, e.Name, lower, e.Name)
	}

	imports := ""
	for _, e := range entities {
		imports += fmt.Sprintf("const %sListPage = lazy(() => import('./pages/%sListPage'));\n", e.Name, e.Name)
		imports += fmt.Sprintf("const %sDetailPage = lazy(() => import('./pages/%sDetailPage'));\n", e.Name, e.Name)
		imports += fmt.Sprintf("const %sCreatePage = lazy(() => import('./pages/%sCreatePage'));\n", e.Name, e.Name)
		imports += fmt.Sprintf("const %sEditPage = lazy(() => import('./pages/%sEditPage'));\n", e.Name, e.Name)
	}

	// Feature-specific imports and routes
	featureImports := ""
	featureRoutes := ""

	if features.HasAuthEmail {
		featureImports += `const ForgotPasswordPage = lazy(() => import('./pages/ForgotPasswordPage'));
const ResetPasswordPage = lazy(() => import('./pages/ResetPasswordPage'));
const VerifyEmailPage = lazy(() => import('./pages/VerifyEmailPage'));
`
		featureRoutes += `        <Route path="/forgot-password" element={<Suspense fallback={<Loading fullScreen />}><ForgotPasswordPage /></Suspense>} />
        <Route path="/reset-password" element={<Suspense fallback={<Loading fullScreen />}><ResetPasswordPage /></Suspense>} />
        <Route path="/verify-email" element={<Suspense fallback={<Loading fullScreen />}><VerifyEmailPage /></Suspense>} />
`
	}

	if features.HasAuthOAuth {
		featureImports += `const OAuthCallbackPage = lazy(() => import('./pages/OAuthCallbackPage'));
const LinkedAccountsPage = lazy(() => import('./pages/LinkedAccountsPage'));
`
		featureRoutes += `        <Route path="/auth/callback/:provider" element={<Suspense fallback={<Loading fullScreen />}><OAuthCallbackPage /></Suspense>} />
        <Route path="/settings/linked-accounts" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><LinkedAccountsPage /></Suspense></Layout></ProtectedRoute>} />
`
	}

	if features.HasStripe {
		featureImports += `const PricingPage = lazy(() => import('./pages/PricingPage'));
const BillingPage = lazy(() => import('./pages/BillingPage'));
const CheckoutSuccessPage = lazy(() => import('./pages/CheckoutSuccessPage'));
`
		featureRoutes += `        <Route path="/pricing" element={<Suspense fallback={<Loading fullScreen />}><PricingPage /></Suspense>} />
        <Route path="/settings/billing" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><BillingPage /></Suspense></Layout></ProtectedRoute>} />
        <Route path="/checkout/success" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><CheckoutSuccessPage /></Suspense></Layout></ProtectedRoute>} />
`
	}

	if features.HasNotification {
		featureImports += `const NotificationsPage = lazy(() => import('./pages/NotificationsPage'));
const NotificationSettingsPage = lazy(() => import('./pages/NotificationSettingsPage'));
`
		featureRoutes += `        <Route path="/notifications" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><NotificationsPage /></Suspense></Layout></ProtectedRoute>} />
        <Route path="/settings/notifications" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><NotificationSettingsPage /></Suspense></Layout></ProtectedRoute>} />
`
	}

	if features.HasGeo {
		featureImports += `const StoreLocatorPage = lazy(() => import('./pages/StoreLocatorPage'));
`
		featureRoutes += `        <Route path="/locations" element={<Suspense fallback={<Loading fullScreen />}><StoreLocatorPage /></Suspense>} />
`
	}

	// Settings and profile always included
	featureImports += `const SettingsPage = lazy(() => import('./pages/SettingsPage'));
const ProfilePage = lazy(() => import('./pages/ProfilePage'));
`
	featureRoutes += `        <Route path="/settings" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><SettingsPage /></Suspense></Layout></ProtectedRoute>} />
        <Route path="/profile" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><ProfilePage /></Suspense></Layout></ProtectedRoute>} />
`

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Suspense, lazy } from 'react';
import { AuthProvider, useAuth } from './context/AuthContext';
import { ToastProvider } from './components/ui/Toast';
import { Loading } from './components/ui/Loading';
import { Layout } from './components/Layout';

// Lazy load pages
const DashboardPage = lazy(() => import('./pages/DashboardPage'));
const LoginPage = lazy(() => import('./pages/LoginPage'));
const SignupPage = lazy(() => import('./pages/SignupPage'));
%s%s
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();
  if (isLoading) return <Loading fullScreen />;
  if (!isAuthenticated) return <Navigate to="/login" />;
  return <>{children}</>;
}

export default function App() {
  return (
    <AuthProvider>
      <ToastProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/login" element={<Suspense fallback={<Loading fullScreen />}><LoginPage /></Suspense>} />
            <Route path="/signup" element={<Suspense fallback={<Loading fullScreen />}><SignupPage /></Suspense>} />
%s            <Route path="/" element={<ProtectedRoute><Layout><Suspense fallback={<Loading />}><DashboardPage /></Suspense></Layout></ProtectedRoute>} />
%s
            <Route path="*" element={<Navigate to="/" />} />
          </Routes>
        </BrowserRouter>
      </ToastProvider>
    </AuthProvider>
  );
}
`, imports, featureImports, featureRoutes, routes)
}

func generateMainTsx() string {
	return `// Generated by protoc-gen-react-app
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
`
}

func generateIndexCss() string {
	return `@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  html {
    @apply antialiased;
  }
  body {
    @apply bg-gray-50 text-gray-900;
  }
}

@layer components {
  .btn {
    @apply inline-flex items-center justify-center px-4 py-2 rounded-lg font-medium transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed;
  }
  .btn-primary {
    @apply bg-indigo-600 text-white hover:bg-indigo-700 focus:ring-indigo-500;
  }
  .btn-secondary {
    @apply bg-white text-gray-700 border border-gray-300 hover:bg-gray-50 focus:ring-indigo-500;
  }
  .btn-danger {
    @apply bg-red-600 text-white hover:bg-red-700 focus:ring-red-500;
  }
  .input {
    @apply block w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-colors;
  }
  .label {
    @apply block text-sm font-medium text-gray-700 mb-1;
  }
  .card {
    @apply bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden;
  }
  .page-header {
    @apply mb-8;
  }
  .page-title {
    @apply text-2xl font-bold text-gray-900;
  }
  .page-subtitle {
    @apply text-gray-500 mt-1;
  }
}

/* Custom scrollbar */
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  @apply bg-gray-100;
}
::-webkit-scrollbar-thumb {
  @apply bg-gray-300 rounded-full;
}
::-webkit-scrollbar-thumb:hover {
  @apply bg-gray-400;
}
`
}

func generateTypes(entities []Entity) string {
	types := "// Generated by protoc-gen-react-app\n\n"

	for _, e := range entities {
		types += fmt.Sprintf("export interface %s {\n", e.Name)
		for _, f := range e.Fields {
			optional := ""
			if !f.IsRequired {
				optional = "?"
			}
			types += fmt.Sprintf("  %s%s: %s;\n", f.Name, optional, f.TsType)
		}
		types += "}\n\n"

		// Input type (without id)
		types += fmt.Sprintf("export interface %sInput {\n", e.Name)
		for _, f := range e.Fields {
			if f.IsId {
				continue
			}
			types += fmt.Sprintf("  %s?: %s;\n", f.Name, f.TsType)
		}
		types += "}\n\n"
	}

	types += `export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export interface ApiError {
  message: string;
  code?: string;
}
`
	return types
}

func generateApi(entities []Entity, features Features) string {
	api := `// Generated by protoc-gen-react-app
const API_BASE = import.meta.env.VITE_API_URL || '/api';

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const token = localStorage.getItem('token');
  const res = await fetch(API_BASE + url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: 'Bearer ' + token } : {}),
      ...options?.headers,
    },
  });
  if (!res.ok) {
    const error = await res.json().catch(() => ({ message: res.statusText }));
    throw new Error(error.message || 'Request failed');
  }
  return res.json();
}

// Auth API
export const authApi = {
  login: (email: string, password: string) => 
    request<{ token: string; user: any }>('/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),
  signup: (name: string, email: string, password: string) => 
    request<{ token: string; user: any }>('/auth/signup', { method: 'POST', body: JSON.stringify({ name, email, password }) }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  me: () => request<any>('/auth/me'),
};

`
	// Entity APIs
	for _, e := range entities {
		lower := strings.ToLower(e.Name)
		api += fmt.Sprintf(`// %s API
export const %sApi = {
  list: (params?: { page?: number; pageSize?: number; search?: string }) => 
    request<%s[]>('/%ss?' + new URLSearchParams(params as any).toString()),
  get: (id: string) => request<%s>('/%ss/' + id),
  create: (data: %sInput) => request<%s>('/%ss', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: %sInput) => request<%s>('/%ss/' + id, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) => request<void>('/%ss/' + id, { method: 'DELETE' }),
};

`, e.Name, lower, e.Name, lower, e.Name, lower, e.Name, e.Name, lower, e.Name, e.Name, lower, lower)
	}

	// Feature-specific APIs
	if features.HasAuthEmail {
		api += `// Auth Email API
export const authEmailApi = {
  forgotPassword: (email: string) => 
    request<void>('/auth/forgot-password', { method: 'POST', body: JSON.stringify({ email }) }),
  resetPassword: (token: string, password: string) => 
    request<void>('/auth/reset-password', { method: 'POST', body: JSON.stringify({ token, password }) }),
  verifyEmail: (token: string) => 
    request<void>('/auth/verify-email', { method: 'POST', body: JSON.stringify({ token }) }),
  resendVerification: () => 
    request<void>('/auth/resend-verification', { method: 'POST' }),
  refreshToken: (refreshToken: string) => 
    request<{ token: string; refreshToken: string }>('/auth/refresh', { method: 'POST', body: JSON.stringify({ refreshToken }) }),
};

`
	}

	if features.HasAuthOAuth {
		api += `// OAuth API
export const oauthApi = {
  startAuth: (provider: string) => 
    request<{ url: string }>('/auth/oauth/' + provider + '/start'),
  callback: (provider: string, code: string, state: string) => 
    request<{ token: string; user: any }>('/auth/oauth/' + provider + '/callback', { method: 'POST', body: JSON.stringify({ code, state }) }),
  getLinkedProviders: () => 
    request<{ providers: string[] }>('/auth/oauth/linked'),
  linkProvider: (provider: string) => 
    request<{ url: string }>('/auth/oauth/' + provider + '/link'),
  unlinkProvider: (provider: string) => 
    request<void>('/auth/oauth/' + provider + '/unlink', { method: 'DELETE' }),
};

`
	}

	if features.HasStripe {
		api += `// Stripe API
export const stripeApi = {
  createCheckoutSession: (priceId: string, mode?: string) => 
    request<{ sessionId: string; url: string }>('/stripe/checkout', { method: 'POST', body: JSON.stringify({ priceId, mode }) }),
  createPortalSession: () => 
    request<{ url: string }>('/stripe/portal', { method: 'POST' }),
  getSubscription: () => 
    request<any>('/stripe/subscription'),
  cancelSubscription: () => 
    request<void>('/stripe/subscription/cancel', { method: 'POST' }),
  reactivateSubscription: () => 
    request<void>('/stripe/subscription/reactivate', { method: 'POST' }),
  getInvoices: () => 
    request<any[]>('/stripe/invoices'),
  getPaymentMethods: () => 
    request<any[]>('/stripe/payment-methods'),
};

`
	}

	if features.HasNotification {
		api += `// Notification API
export const notificationApi = {
  list: (params?: { limit?: number; unreadOnly?: boolean }) => 
    request<any[]>('/notifications?' + new URLSearchParams(params as any).toString()),
  markAsRead: (id: string) => 
    request<void>('/notifications/' + id + '/read', { method: 'POST' }),
  markAllAsRead: () => 
    request<void>('/notifications/read-all', { method: 'POST' }),
  getUnreadCount: () => 
    request<{ count: number }>('/notifications/unread-count'),
  getPreferences: () => 
    request<any>('/notifications/preferences'),
  updatePreferences: (prefs: any) => 
    request<void>('/notifications/preferences', { method: 'PUT', body: JSON.stringify(prefs) }),
  updateFcmToken: (token: string) => 
    request<void>('/notifications/fcm-token', { method: 'PUT', body: JSON.stringify({ token }) }),
  testNotification: () => 
    request<void>('/notifications/test', { method: 'POST' }),
};

`
	}

	if features.HasGeo {
		api += `// Geo/Location API
export const geoApi = {
  findNearest: (lat: number, lng: number, radiusKm?: number, limit?: number) => 
    request<any[]>('/locations/nearest?' + new URLSearchParams({ lat: String(lat), lng: String(lng), radiusKm: String(radiusKm || 10), limit: String(limit || 20) }).toString()),
  findInBounds: (minLat: number, maxLat: number, minLng: number, maxLng: number) => 
    request<any[]>('/locations/bounds?' + new URLSearchParams({ minLat: String(minLat), maxLat: String(maxLat), minLng: String(minLng), maxLng: String(maxLng) }).toString()),
  search: (query: string) => 
    request<any[]>('/locations/search?q=' + encodeURIComponent(query)),
};

`
	}

	return api
}

func generateHooks(entities []Entity) string {
	hooks := `// Generated by protoc-gen-react-app
import { useState, useEffect, useCallback } from 'react';
import { useToast } from './components/ui/Toast';

`
	for _, e := range entities {
		lower := strings.ToLower(e.Name)
		hooks += fmt.Sprintf(`import { %sApi } from './api';
import type { %s, %sInput } from './types';

export function use%sList() {
  const [items, setItems] = useState<%s[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { showToast } = useToast();

  const fetch = useCallback(async (params?: { search?: string }) => {
    setLoading(true);
    try {
      const data = await %sApi.list(params);
      setItems(data);
      setError(null);
    } catch (e: any) {
      setError(e.message);
      showToast(e.message, 'error');
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => { fetch(); }, [fetch]);

  return { items, loading, error, refetch: fetch };
}

export function use%s(id: string | undefined) {
  const [item, setItem] = useState<%s | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    %sApi.get(id)
      .then(setItem)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [id]);

  return { item, loading, error };
}

export function use%sMutations() {
  const [loading, setLoading] = useState(false);
  const { showToast } = useToast();

  const create = async (data: %sInput) => {
    setLoading(true);
    try {
      const result = await %sApi.create(data);
      showToast('%s created successfully', 'success');
      return result;
    } catch (e: any) {
      showToast(e.message, 'error');
      throw e;
    } finally {
      setLoading(false);
    }
  };

  const update = async (id: string, data: %sInput) => {
    setLoading(true);
    try {
      const result = await %sApi.update(id, data);
      showToast('%s updated successfully', 'success');
      return result;
    } catch (e: any) {
      showToast(e.message, 'error');
      throw e;
    } finally {
      setLoading(false);
    }
  };

  const remove = async (id: string) => {
    setLoading(true);
    try {
      await %sApi.delete(id);
      showToast('%s deleted successfully', 'success');
    } catch (e: any) {
      showToast(e.message, 'error');
      throw e;
    } finally {
      setLoading(false);
    }
  };

  return { create, update, remove, loading };
}

`, lower, e.Name, e.Name, e.Name, e.Name, lower, e.Name, e.Name, lower, e.Name, e.Name, lower, e.Name, e.Name, lower, e.Name, lower, e.Name)
	}

	return hooks
}

// =============================================================================
// LAYOUT COMPONENTS
// =============================================================================

func generateLayout(entities []Entity) string {
	return `// Generated by protoc-gen-react-app
import { Sidebar } from './Sidebar';
import { Topbar } from './Topbar';

interface LayoutProps {
  children: React.ReactNode;
}

export function Layout({ children }: LayoutProps) {
  return (
    <div className="min-h-screen flex">
      <Sidebar />
      <div className="flex-1 flex flex-col ml-64">
        <Topbar />
        <main className="flex-1 p-6 overflow-auto">
          {children}
        </main>
      </div>
    </div>
  );
}
`
}

func generateSidebar(entities []Entity, features Features) string {
	navItems := ""
	for _, e := range entities {
		lower := strings.ToLower(e.Name)
		icon := getIconForEntity(e.Name)
		navItems += fmt.Sprintf(`    { name: '%s', href: '/%s', icon: '%s' },
`, e.Name+"s", lower, icon)
	}

	// Feature nav sections
	settingsItems := `  { name: 'Profile', href: '/profile', icon: '👤' },
`
	if features.HasStripe {
		settingsItems += `  { name: 'Billing', href: '/settings/billing', icon: '💳' },
`
	}
	if features.HasNotification {
		settingsItems += `  { name: 'Notifications', href: '/settings/notifications', icon: '🔔' },
`
	}
	if features.HasAuthOAuth {
		settingsItems += `  { name: 'Linked Accounts', href: '/settings/linked-accounts', icon: '🔗' },
`
	}

	publicItems := ""
	if features.HasStripe {
		publicItems += `  { name: 'Pricing', href: '/pricing', icon: '💰', public: true },
`
	}
	if features.HasGeo {
		publicItems += `  { name: 'Store Locator', href: '/locations', icon: '📍', public: true },
`
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';

const mainNavigation = [
  { name: 'Dashboard', href: '/', icon: '📊' },
%s];

const settingsNavigation = [
%s];

const publicNavigation = [
%s];

export function Sidebar() {
  const location = useLocation();
  const { user } = useAuth();

  const isActive = (href: string) => 
    location.pathname === href || (href !== '/' && location.pathname.startsWith(href));

  const NavLink = ({ item }: { item: { name: string; href: string; icon: string } }) => (
    <Link
      to={item.href}
      className={"flex items-center px-3 py-2 rounded-lg text-sm font-medium transition-colors " +
        (isActive(item.href) 
          ? "bg-indigo-600 text-white" 
          : "text-gray-300 hover:bg-gray-800 hover:text-white"
        )}
    >
      <span className="mr-3 text-lg">{item.icon}</span>
      {item.name}
    </Link>
  );

  return (
    <aside className="fixed inset-y-0 left-0 w-64 bg-gray-900 text-white flex flex-col">
      <div className="flex items-center h-16 px-6 border-b border-gray-800">
        <span className="text-xl font-bold">Admin</span>
      </div>
      
      <nav className="flex-1 overflow-y-auto py-4 px-3">
        <div className="space-y-1">
          {mainNavigation.map((item) => <NavLink key={item.href} item={item} />)}
        </div>
        
        {publicNavigation.length > 0 && (
          <>
            <div className="mt-8 mb-2 px-3 text-xs font-semibold text-gray-400 uppercase">Public</div>
            <div className="space-y-1">
              {publicNavigation.map((item) => <NavLink key={item.href} item={item} />)}
            </div>
          </>
        )}
        
        <div className="mt-8 mb-2 px-3 text-xs font-semibold text-gray-400 uppercase">Settings</div>
        <div className="space-y-1">
          {settingsNavigation.map((item) => <NavLink key={item.href} item={item} />)}
        </div>
      </nav>
      
      <div className="p-4 border-t border-gray-800">
        <div className="flex items-center">
          <div className="w-10 h-10 bg-indigo-600 rounded-full flex items-center justify-center text-sm font-medium">
            {user?.name?.[0]?.toUpperCase() || 'A'}
          </div>
          <div className="ml-3 overflow-hidden">
            <p className="text-sm font-medium truncate">{user?.name || 'Admin User'}</p>
            <p className="text-xs text-gray-400 truncate">{user?.email || 'admin@example.com'}</p>
          </div>
        </div>
      </div>
    </aside>
  );
}
`, navItems, settingsItems, publicItems)
}

func getIconForEntity(name string) string {
	icons := map[string]string{
		"User": "👤", "Account": "👤", "Profile": "👤",
		"Product": "📦", "Item": "📦", "Inventory": "📦",
		"Order": "🛒", "Cart": "🛒", "Purchase": "🛒",
		"Post": "📝", "Article": "📝", "Blog": "📝",
		"Comment": "💬", "Message": "💬", "Chat": "💬",
		"Category": "📁", "Folder": "📁", "Group": "📁",
		"Tag": "🏷️", "Label": "🏷️",
		"Setting": "⚙️", "Config": "⚙️",
		"Report": "📈", "Analytics": "📈", "Stats": "📈",
		"File": "📄", "Document": "📄",
		"Image": "🖼️", "Photo": "🖼️", "Media": "🖼️",
		"Event": "📅", "Calendar": "📅", "Schedule": "📅",
		"Task": "✅", "Todo": "✅",
		"Project": "🎯",
		"Team":    "👥", "Organization": "👥", "Company": "👥",
		"Invoice": "🧾", "Payment": "💳", "Transaction": "💳",
		"Notification": "🔔", "Alert": "🔔",
		"Location": "📍", "Address": "📍", "Store": "🏪",
	}
	if icon, ok := icons[name]; ok {
		return icon
	}
	return "📋"
}

func generateTopbar(features Features) string {
	notificationBell := ""
	if features.HasNotification {
		notificationBell = `
        <NotificationBell />`
	} else {
		notificationBell = `
        <button className="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg relative">
          🔔
        </button>`
	}

	notificationImport := ""
	if features.HasNotification {
		notificationImport = "\nimport { NotificationBell } from './NotificationBell';"
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { Breadcrumb } from './Breadcrumb';
import { Dropdown } from './ui/Dropdown';
import { SearchInput } from './ui/SearchInput';%s

export function Topbar() {
  const { logout, user } = useAuth();
  const navigate = useNavigate();

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  return (
    <header className="h-16 bg-white border-b border-gray-200 flex items-center justify-between px-6">
      <div className="flex items-center gap-4">
        <Breadcrumb />
      </div>
      
      <div className="flex items-center gap-4">
        <SearchInput 
          placeholder="Search..." 
          className="w-64"
          onSearch={(q) => console.log('Search:', q)}
        />
        %s
        
        <Dropdown
          trigger={
            <button className="flex items-center gap-2 p-2 hover:bg-gray-100 rounded-lg">
              <div className="w-8 h-8 bg-indigo-600 rounded-full flex items-center justify-center text-white text-sm font-medium">
                {user?.name?.[0] || 'A'}
              </div>
            </button>
          }
          items={[
            { label: 'Profile', onClick: () => navigate('/profile') },
            { label: 'Settings', onClick: () => navigate('/settings') },
            { type: 'divider' },
            { label: 'Logout', onClick: handleLogout, className: 'text-red-600' },
          ]}
        />
      </div>
    </header>
  );
}
`, notificationImport, notificationBell)
}

func generateBreadcrumb() string {
	return `// Generated by protoc-gen-react-app
import { Link, useLocation } from 'react-router-dom';

export function Breadcrumb() {
  const location = useLocation();
  const paths = location.pathname.split('/').filter(Boolean);

  if (paths.length === 0) {
    return <h1 className="text-lg font-semibold">Dashboard</h1>;
  }

  return (
    <nav className="flex items-center gap-2 text-sm">
      <Link to="/" className="text-gray-500 hover:text-gray-700">Home</Link>
      {paths.map((path, i) => {
        const href = '/' + paths.slice(0, i + 1).join('/');
        const isLast = i === paths.length - 1;
        const label = path.charAt(0).toUpperCase() + path.slice(1);

        return (
          <span key={path} className="flex items-center gap-2">
            <span className="text-gray-300">/</span>
            {isLast ? (
              <span className="text-gray-900 font-medium">{label}</span>
            ) : (
              <Link to={href} className="text-gray-500 hover:text-gray-700">{label}</Link>
            )}
          </span>
        );
      })}
    </nav>
  );
}
`
}

// =============================================================================
// UI COMPONENTS
// =============================================================================

func generateButton() string {
	return `// Generated by protoc-gen-react-app
import { forwardRef } from 'react';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  size?: 'sm' | 'md' | 'lg';
  loading?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = 'primary', size = 'md', loading, children, className = '', disabled, ...props }, ref) => {
    const variants = {
      primary: 'bg-indigo-600 text-white hover:bg-indigo-700 focus:ring-indigo-500',
      secondary: 'bg-white text-gray-700 border border-gray-300 hover:bg-gray-50 focus:ring-indigo-500',
      danger: 'bg-red-600 text-white hover:bg-red-700 focus:ring-red-500',
      ghost: 'bg-transparent text-gray-700 hover:bg-gray-100 focus:ring-gray-500',
    };

    const sizes = {
      sm: 'px-3 py-1.5 text-sm',
      md: 'px-4 py-2',
      lg: 'px-6 py-3 text-lg',
    };

    return (
      <button
        ref={ref}
        disabled={disabled || loading}
        className={"btn " + variants[variant] + " " + sizes[size] + " " + className}
        {...props}
      >
        {loading && (
          <svg className="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
        )}
        {children}
      </button>
    );
  }
);
`
}

func generateInput() string {
	return `// Generated by protoc-gen-react-app
import { forwardRef } from 'react';

interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  hint?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ label, error, hint, className = '', ...props }, ref) => {
    return (
      <div className="w-full">
        {label && <label className="label">{label}</label>}
        <input
          ref={ref}
          className={"input " + (error ? 'border-red-500 focus:ring-red-500' : '') + " " + className}
          {...props}
        />
        {hint && !error && <p className="mt-1 text-sm text-gray-500">{hint}</p>}
        {error && <p className="mt-1 text-sm text-red-600">{error}</p>}
      </div>
    );
  }
);

interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  error?: string;
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ label, error, className = '', ...props }, ref) => {
    return (
      <div className="w-full">
        {label && <label className="label">{label}</label>}
        <textarea
          ref={ref}
          className={"input min-h-[100px] " + (error ? 'border-red-500' : '') + " " + className}
          {...props}
        />
        {error && <p className="mt-1 text-sm text-red-600">{error}</p>}
      </div>
    );
  }
);

interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  error?: string;
  options: { value: string; label: string }[];
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ label, error, options, className = '', ...props }, ref) => {
    return (
      <div className="w-full">
        {label && <label className="label">{label}</label>}
        <select ref={ref} className={"input " + className} {...props}>
          <option value="">Select...</option>
          {options.map(opt => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
        {error && <p className="mt-1 text-sm text-red-600">{error}</p>}
      </div>
    );
  }
);

interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label: string;
}

export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(
  ({ label, className = '', ...props }, ref) => {
    return (
      <label className="flex items-center gap-2 cursor-pointer">
        <input ref={ref} type="checkbox" className={"w-4 h-4 text-indigo-600 rounded " + className} {...props} />
        <span className="text-sm text-gray-700">{label}</span>
      </label>
    );
  }
);
`
}

func generateTable() string {
	return `// Generated by protoc-gen-react-app
interface Column<T> {
  key: keyof T | string;
  header: string;
  render?: (item: T) => React.ReactNode;
  className?: string;
}

interface TableProps<T> {
  columns: Column<T>[];
  data: T[];
  onRowClick?: (item: T) => void;
  loading?: boolean;
  emptyMessage?: string;
}

export function Table<T extends { id: string }>({ columns, data, onRowClick, loading, emptyMessage = 'No data found' }: TableProps<T>) {
  if (loading) {
    return (
      <div className="card">
        <div className="animate-pulse p-4 space-y-3">
          {[1, 2, 3, 4, 5].map(i => (
            <div key={i} className="h-12 bg-gray-100 rounded" />
          ))}
        </div>
      </div>
    );
  }

  if (data.length === 0) {
    return (
      <div className="card p-8 text-center text-gray-500">
        {emptyMessage}
      </div>
    );
  }

  return (
    <div className="card overflow-hidden">
      <table className="w-full">
        <thead className="bg-gray-50 border-b border-gray-200">
          <tr>
            {columns.map(col => (
              <th key={String(col.key)} className={"px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider " + (col.className || '')}>
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-200">
          {data.map(item => (
            <tr 
              key={item.id} 
              onClick={() => onRowClick?.(item)}
              className={"hover:bg-gray-50 " + (onRowClick ? 'cursor-pointer' : '')}
            >
              {columns.map(col => (
                <td key={String(col.key)} className={"px-4 py-3 text-sm text-gray-900 " + (col.className || '')}>
                  {col.render ? col.render(item) : String((item as any)[col.key] ?? '-')}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
`
}

func generateModal() string {
	return `// Generated by protoc-gen-react-app
import { useEffect } from 'react';

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: string;
  children: React.ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
}

export function Modal({ isOpen, onClose, title, children, size = 'md' }: ModalProps) {
  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    if (isOpen) {
      document.addEventListener('keydown', handleEsc);
      document.body.style.overflow = 'hidden';
    }
    return () => {
      document.removeEventListener('keydown', handleEsc);
      document.body.style.overflow = '';
    };
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const sizes = {
    sm: 'max-w-md',
    md: 'max-w-lg',
    lg: 'max-w-2xl',
    xl: 'max-w-4xl',
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="fixed inset-0 bg-black/50" onClick={onClose} />
      <div className={"relative bg-white rounded-xl shadow-xl w-full " + sizes[size]}>
        {title && (
          <div className="flex items-center justify-between px-6 py-4 border-b">
            <h2 className="text-lg font-semibold">{title}</h2>
            <button onClick={onClose} className="text-gray-400 hover:text-gray-600">✕</button>
          </div>
        )}
        <div className="p-6">{children}</div>
      </div>
    </div>
  );
}

interface ConfirmModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  variant?: 'danger' | 'warning';
}

export function ConfirmModal({ isOpen, onClose, onConfirm, title, message, confirmText = 'Confirm', cancelText = 'Cancel', variant = 'danger' }: ConfirmModalProps) {
  return (
    <Modal isOpen={isOpen} onClose={onClose} size="sm">
      <div className="text-center">
        <div className={"mx-auto w-12 h-12 rounded-full flex items-center justify-center mb-4 " + (variant === 'danger' ? 'bg-red-100' : 'bg-yellow-100')}>
          {variant === 'danger' ? '⚠️' : '❓'}
        </div>
        <h3 className="text-lg font-semibold mb-2">{title}</h3>
        <p className="text-gray-500 mb-6">{message}</p>
        <div className="flex gap-3 justify-center">
          <button onClick={onClose} className="btn btn-secondary">{cancelText}</button>
          <button onClick={() => { onConfirm(); onClose(); }} className={"btn " + (variant === 'danger' ? 'btn-danger' : 'btn-primary')}>{confirmText}</button>
        </div>
      </div>
    </Modal>
  );
}
`
}

func generateCard() string {
	return `// Generated by protoc-gen-react-app
interface CardProps {
  children: React.ReactNode;
  className?: string;
  padding?: boolean;
}

export function Card({ children, className = '', padding = true }: CardProps) {
  return (
    <div className={"card " + (padding ? 'p-6' : '') + " " + className}>
      {children}
    </div>
  );
}

interface CardHeaderProps {
  title: string;
  subtitle?: string;
  action?: React.ReactNode;
}

export function CardHeader({ title, subtitle, action }: CardHeaderProps) {
  return (
    <div className="flex items-center justify-between mb-4">
      <div>
        <h3 className="text-lg font-semibold">{title}</h3>
        {subtitle && <p className="text-sm text-gray-500">{subtitle}</p>}
      </div>
      {action}
    </div>
  );
}

interface StatCardProps {
  title: string;
  value: string | number;
  change?: { value: number; positive: boolean };
  icon?: string;
}

export function StatCard({ title, value, change, icon }: StatCardProps) {
  return (
    <Card>
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-gray-500">{title}</p>
          <p className="text-2xl font-bold mt-1">{value}</p>
          {change && (
            <p className={"text-sm mt-1 " + (change.positive ? 'text-green-600' : 'text-red-600')}>
              {change.positive ? '↑' : '↓'} {Math.abs(change.value)}%
            </p>
          )}
        </div>
        {icon && <span className="text-3xl opacity-50">{icon}</span>}
      </div>
    </Card>
  );
}
`
}

func generateBadge() string {
	return `// Generated by protoc-gen-react-app
interface BadgeProps {
  children: React.ReactNode;
  variant?: 'default' | 'success' | 'warning' | 'danger' | 'info';
  size?: 'sm' | 'md';
}

export function Badge({ children, variant = 'default', size = 'md' }: BadgeProps) {
  const variants = {
    default: 'bg-gray-100 text-gray-800',
    success: 'bg-green-100 text-green-800',
    warning: 'bg-yellow-100 text-yellow-800',
    danger: 'bg-red-100 text-red-800',
    info: 'bg-blue-100 text-blue-800',
  };

  const sizes = {
    sm: 'px-2 py-0.5 text-xs',
    md: 'px-2.5 py-1 text-sm',
  };

  return (
    <span className={"inline-flex items-center font-medium rounded-full " + variants[variant] + " " + sizes[size]}>
      {children}
    </span>
  );
}
`
}

func generateDropdown() string {
	return `// Generated by protoc-gen-react-app
import { useState, useRef, useEffect } from 'react';

interface DropdownItem {
  label?: string;
  onClick?: () => void;
  type?: 'divider';
  className?: string;
}

interface DropdownProps {
  trigger: React.ReactNode;
  items: DropdownItem[];
}

export function Dropdown({ trigger, items }: DropdownProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  return (
    <div ref={ref} className="relative">
      <div onClick={() => setOpen(!open)}>{trigger}</div>
      {open && (
        <div className="absolute right-0 mt-2 w-48 bg-white rounded-lg shadow-lg border py-1 z-50">
          {items.map((item, i) => 
            item.type === 'divider' ? (
              <div key={i} className="border-t my-1" />
            ) : (
              <button
                key={i}
                onClick={() => { item.onClick?.(); setOpen(false); }}
                className={"w-full px-4 py-2 text-left text-sm hover:bg-gray-50 " + (item.className || '')}
              >
                {item.label}
              </button>
            )
          )}
        </div>
      )}
    </div>
  );
}
`
}

func generateToast() string {
	return `// Generated by protoc-gen-react-app
import { createContext, useContext, useState, useCallback } from 'react';

type ToastType = 'success' | 'error' | 'warning' | 'info';

interface Toast {
  id: number;
  message: string;
  type: ToastType;
}

interface ToastContextType {
  showToast: (message: string, type?: ToastType) => void;
}

const ToastContext = createContext<ToastContextType | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const showToast = useCallback((message: string, type: ToastType = 'info') => {
    const id = Date.now();
    setToasts(t => [...t, { id, message, type }]);
    setTimeout(() => setToasts(t => t.filter(x => x.id !== id)), 5000);
  }, []);

  const colors = {
    success: 'bg-green-500',
    error: 'bg-red-500',
    warning: 'bg-yellow-500',
    info: 'bg-blue-500',
  };

  const icons = {
    success: '✓',
    error: '✕',
    warning: '⚠',
    info: 'ℹ',
  };

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      <div className="fixed bottom-4 right-4 z-50 space-y-2">
        {toasts.map(toast => (
          <div key={toast.id} className={"flex items-center gap-3 px-4 py-3 rounded-lg text-white shadow-lg " + colors[toast.type]}>
            <span>{icons[toast.type]}</span>
            <span>{toast.message}</span>
            <button onClick={() => setToasts(t => t.filter(x => x.id !== toast.id))} className="ml-2 opacity-70 hover:opacity-100">✕</button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast requires ToastProvider');
  return ctx;
}
`
}

func generateLoading() string {
	return `// Generated by protoc-gen-react-app
interface LoadingProps {
  fullScreen?: boolean;
  size?: 'sm' | 'md' | 'lg';
}

export function Loading({ fullScreen, size = 'md' }: LoadingProps) {
  const sizes = { sm: 'w-4 h-4', md: 'w-8 h-8', lg: 'w-12 h-12' };

  const spinner = (
    <svg className={"animate-spin text-indigo-600 " + sizes[size]} fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
    </svg>
  );

  if (fullScreen) {
    return (
      <div className="fixed inset-0 bg-white flex items-center justify-center">
        {spinner}
      </div>
    );
  }

  return <div className="flex items-center justify-center p-8">{spinner}</div>;
}
`
}

func generateEmptyState() string {
	return `// Generated by protoc-gen-react-app
interface EmptyStateProps {
  icon?: string;
  title: string;
  description?: string;
  action?: React.ReactNode;
}

export function EmptyState({ icon = '📭', title, description, action }: EmptyStateProps) {
  return (
    <div className="text-center py-12">
      <span className="text-5xl">{icon}</span>
      <h3 className="mt-4 text-lg font-medium text-gray-900">{title}</h3>
      {description && <p className="mt-2 text-gray-500">{description}</p>}
      {action && <div className="mt-6">{action}</div>}
    </div>
  );
}
`
}

func generatePagination() string {
	return `// Generated by protoc-gen-react-app
interface PaginationProps {
  currentPage: number;
  totalPages: number;
  onPageChange: (page: number) => void;
}

export function Pagination({ currentPage, totalPages, onPageChange }: PaginationProps) {
  if (totalPages <= 1) return null;

  const pages = [];
  for (let i = 1; i <= totalPages; i++) {
    if (i === 1 || i === totalPages || (i >= currentPage - 1 && i <= currentPage + 1)) {
      pages.push(i);
    } else if (pages[pages.length - 1] !== '...') {
      pages.push('...');
    }
  }

  return (
    <div className="flex items-center justify-center gap-1 mt-6">
      <button
        onClick={() => onPageChange(currentPage - 1)}
        disabled={currentPage === 1}
        className="px-3 py-2 rounded-lg text-sm disabled:opacity-50 hover:bg-gray-100"
      >
        ← Prev
      </button>
      {pages.map((page, i) => 
        page === '...' ? (
          <span key={i} className="px-3 py-2">...</span>
        ) : (
          <button
            key={i}
            onClick={() => onPageChange(page as number)}
            className={"px-3 py-2 rounded-lg text-sm " + (currentPage === page ? 'bg-indigo-600 text-white' : 'hover:bg-gray-100')}
          >
            {page}
          </button>
        )
      )}
      <button
        onClick={() => onPageChange(currentPage + 1)}
        disabled={currentPage === totalPages}
        className="px-3 py-2 rounded-lg text-sm disabled:opacity-50 hover:bg-gray-100"
      >
        Next →
      </button>
    </div>
  );
}
`
}

func generateSearchInput() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';

interface SearchInputProps {
  placeholder?: string;
  className?: string;
  onSearch: (query: string) => void;
  debounce?: number;
}

export function SearchInput({ placeholder = 'Search...', className = '', onSearch, debounce = 300 }: SearchInputProps) {
  const [value, setValue] = useState('');

  useEffect(() => {
    const timer = setTimeout(() => onSearch(value), debounce);
    return () => clearTimeout(timer);
  }, [value, debounce, onSearch]);

  return (
    <div className={"relative " + className}>
      <span className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400">🔍</span>
      <input
        type="text"
        value={value}
        onChange={e => setValue(e.target.value)}
        placeholder={placeholder}
        className="input pl-10"
      />
      {value && (
        <button onClick={() => setValue('')} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600">
          ✕
        </button>
      )}
    </div>
  );
}
`
}

func generateUiIndex() string {
	return `// Generated by protoc-gen-react-app
export * from './Button';
export * from './Input';
export * from './Table';
export * from './Modal';
export * from './Card';
export * from './Badge';
export * from './Dropdown';
export * from './Toast';
export * from './Loading';
export * from './EmptyState';
export * from './Pagination';
export * from './SearchInput';
`
}

// =============================================================================
// ENTITY PAGES
// =============================================================================

func generateListPage(e Entity) string {
	lower := strings.ToLower(e.Name)
	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { use%sList, use%sMutations } from '../hooks';
import { %sTable } from '../components/%sTable';
import { Button } from '../components/ui/Button';
import { SearchInput } from '../components/ui/SearchInput';
import { ConfirmModal } from '../components/ui/Modal';
import { EmptyState } from '../components/ui/EmptyState';

export default function %sListPage() {
  const navigate = useNavigate();
  const { items, loading, refetch } = use%sList();
  const { remove, loading: deleting } = use%sMutations();
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  const filtered = items.filter(item => 
    JSON.stringify(item).toLowerCase().includes(search.toLowerCase())
  );

  const handleDelete = async () => {
    if (deleteId) {
      await remove(deleteId);
      setDeleteId(null);
      refetch();
    }
  };

  return (
    <div>
      <div className="page-header flex items-center justify-between">
        <div>
          <h1 className="page-title">%ss</h1>
          <p className="page-subtitle">{items.length} total %ss</p>
        </div>
        <Button onClick={() => navigate('/%s/new')}>+ Add %s</Button>
      </div>

      <div className="mb-6">
        <SearchInput onSearch={setSearch} placeholder="Search %ss..." className="max-w-md" />
      </div>

      {!loading && filtered.length === 0 ? (
        <EmptyState
          icon="%s"
          title="No %ss yet"
          description="Get started by creating your first %s."
          action={<Button onClick={() => navigate('/%s/new')}>Create %s</Button>}
        />
      ) : (
        <%sTable
          data={filtered}
          loading={loading}
          onView={(item) => navigate('/%s/' + item.id)}
          onEdit={(item) => navigate('/%s/' + item.id + '/edit')}
          onDelete={(item) => setDeleteId(item.id)}
        />
      )}

      <ConfirmModal
        isOpen={!!deleteId}
        onClose={() => setDeleteId(null)}
        onConfirm={handleDelete}
        title="Delete %s"
        message="Are you sure you want to delete this %s? This action cannot be undone."
        confirmText={deleting ? 'Deleting...' : 'Delete'}
      />
    </div>
  );
}
`, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, lower, lower, e.Name, lower, getIconForEntity(e.Name), e.Name, lower, lower, e.Name, e.Name, lower, lower, e.Name, lower)
}

func generateDetailPage(e Entity) string {
	lower := strings.ToLower(e.Name)

	fields := ""
	for _, f := range e.Fields {
		if f.IsId {
			continue
		}
		fields += fmt.Sprintf(`          <div>
            <dt className="text-sm text-gray-500">%s</dt>
            <dd className="mt-1 text-gray-900">{item.%s ?? '-'}</dd>
          </div>
`, f.GoName, f.Name)
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { use%s, use%sMutations } from '../hooks';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Loading } from '../components/ui/Loading';
import { Badge } from '../components/ui/Badge';
import { ConfirmModal } from '../components/ui/Modal';

export default function %sDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { item, loading, error } = use%s(id);
  const { remove, loading: deleting } = use%sMutations();
  const [showDelete, setShowDelete] = useState(false);

  if (loading) return <Loading />;
  if (error || !item) return <div className="text-red-500">Error: {error || 'Not found'}</div>;

  const handleDelete = async () => {
    await remove(item.id);
    navigate('/%s');
  };

  return (
    <div>
      <div className="page-header flex items-center justify-between">
        <div>
          <h1 className="page-title">%s Details</h1>
          <p className="page-subtitle">ID: {item.id}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => navigate('/%s')}>← Back</Button>
          <Button variant="secondary" onClick={() => navigate('/%s/' + item.id + '/edit')}>Edit</Button>
          <Button variant="danger" onClick={() => setShowDelete(true)}>Delete</Button>
        </div>
      </div>

      <Card>
        <dl className="grid grid-cols-1 md:grid-cols-2 gap-6">
%s        </dl>
      </Card>

      <ConfirmModal
        isOpen={showDelete}
        onClose={() => setShowDelete(false)}
        onConfirm={handleDelete}
        title="Delete %s"
        message="Are you sure? This cannot be undone."
        confirmText={deleting ? 'Deleting...' : 'Delete'}
      />
    </div>
  );
}
`, e.Name, e.Name, e.Name, e.Name, e.Name, lower, e.Name, lower, lower, fields, e.Name)
}

func generateCreatePage(e Entity) string {
	lower := strings.ToLower(e.Name)
	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useNavigate } from 'react-router-dom';
import { use%sMutations } from '../hooks';
import { %sForm } from '../components/%sForm';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import type { %sInput } from '../types';

export default function %sCreatePage() {
  const navigate = useNavigate();
  const { create, loading } = use%sMutations();

  const handleSubmit = async (data: %sInput) => {
    const result = await create(data);
    navigate('/%s/' + result.id);
  };

  return (
    <div>
      <div className="page-header flex items-center justify-between">
        <div>
          <h1 className="page-title">Create %s</h1>
          <p className="page-subtitle">Add a new %s to the system</p>
        </div>
        <Button variant="secondary" onClick={() => navigate('/%s')}>← Back</Button>
      </div>

      <Card className="max-w-2xl">
        <%sForm onSubmit={handleSubmit} loading={loading} />
      </Card>
    </div>
  );
}
`, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, lower, e.Name, lower, lower, e.Name)
}

func generateEditPage(e Entity) string {
	lower := strings.ToLower(e.Name)
	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useParams, useNavigate } from 'react-router-dom';
import { use%s, use%sMutations } from '../hooks';
import { %sForm } from '../components/%sForm';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Loading } from '../components/ui/Loading';
import type { %sInput } from '../types';

export default function %sEditPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { item, loading: fetching, error } = use%s(id);
  const { update, loading } = use%sMutations();

  if (fetching) return <Loading />;
  if (error || !item) return <div className="text-red-500">Error: {error || 'Not found'}</div>;

  const handleSubmit = async (data: %sInput) => {
    await update(item.id, data);
    navigate('/%s/' + item.id);
  };

  return (
    <div>
      <div className="page-header flex items-center justify-between">
        <div>
          <h1 className="page-title">Edit %s</h1>
          <p className="page-subtitle">ID: {item.id}</p>
        </div>
        <Button variant="secondary" onClick={() => navigate('/%s/' + item.id)}>← Back</Button>
      </div>

      <Card className="max-w-2xl">
        <%sForm initialData={item} onSubmit={handleSubmit} loading={loading} />
      </Card>
    </div>
  );
}
`, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, lower, e.Name, lower, e.Name)
}

func generateForm(e Entity) string {
	fields := ""
	defaultValues := ""

	for _, f := range e.Fields {
		if f.IsId {
			continue
		}

		defaultValues += fmt.Sprintf("    %s: initialData?.%s ?? %s,\n", f.Name, f.Name, getDefaultValue(f))

		if f.InputType == "checkbox" {
			fields += fmt.Sprintf(`      <Checkbox
        label="%s"
        checked={form.%s}
        onChange={e => setForm(f => ({ ...f, %s: e.target.checked }))}
      />
`, f.GoName, f.Name, f.Name)
		} else if f.InputType == "select" && len(f.EnumValues) > 0 {
			options := ""
			for _, v := range f.EnumValues {
				options += fmt.Sprintf("        { value: '%s', label: '%s' },\n", v, v)
			}
			fields += fmt.Sprintf(`      <Select
        label="%s"
        value={form.%s}
        onChange={e => setForm(f => ({ ...f, %s: e.target.value }))}
        options={[
%s        ]}
      />
`, f.GoName, f.Name, f.Name, options)
		} else if f.InputType == "number" {
			fields += fmt.Sprintf(`      <Input
        label="%s"
        type="number"
        value={form.%s}
        onChange={e => setForm(f => ({ ...f, %s: Number(e.target.value) }))}
      />
`, f.GoName, f.Name, f.Name)
		} else {
			fields += fmt.Sprintf(`      <Input
        label="%s"
        type="%s"
        value={form.%s}
        onChange={e => setForm(f => ({ ...f, %s: e.target.value }))}
      />
`, f.GoName, f.InputType, f.Name, f.Name)
		}
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useState } from 'react';
import { Input, Select, Checkbox } from './ui/Input';
import { Button } from './ui/Button';
import type { %s, %sInput } from '../types';

interface %sFormProps {
  initialData?: %s;
  onSubmit: (data: %sInput) => Promise<void>;
  loading?: boolean;
}

export function %sForm({ initialData, onSubmit, loading }: %sFormProps) {
  const [form, setForm] = useState<%sInput>({
%s  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onSubmit(form);
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
%s
      <div className="flex gap-3 pt-4">
        <Button type="submit" loading={loading}>
          {initialData ? 'Save Changes' : 'Create %s'}
        </Button>
      </div>
    </form>
  );
}
`, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, defaultValues, fields, e.Name)
}

func getDefaultValue(f Field) string {
	if f.IsList {
		return "[]"
	}
	switch f.TsType {
	case "boolean":
		return "false"
	case "number":
		return "0"
	default:
		return "''"
	}
}

func generateEntityTable(e Entity) string {
	columns := ""
	for i, f := range e.Fields {
		if i > 5 {
			break // Max 6 columns
		}
		if f.TsType == "boolean" {
			columns += fmt.Sprintf(`    { key: '%s', header: '%s', render: (item) => item.%s ? '✓' : '✗' },
`, f.Name, f.GoName, f.Name)
		} else {
			columns += fmt.Sprintf(`    { key: '%s', header: '%s' },
`, f.Name, f.GoName)
		}
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { Table } from './ui/Table';
import { Button } from './ui/Button';
import { Dropdown } from './ui/Dropdown';
import type { %s } from '../types';

interface %sTableProps {
  data: %s[];
  loading?: boolean;
  onView?: (item: %s) => void;
  onEdit?: (item: %s) => void;
  onDelete?: (item: %s) => void;
}

export function %sTable({ data, loading, onView, onEdit, onDelete }: %sTableProps) {
  const columns = [
%s    {
      key: 'actions',
      header: '',
      className: 'w-12',
      render: (item: %s) => (
        <Dropdown
          trigger={<button className="p-1 hover:bg-gray-100 rounded">⋯</button>}
          items={[
            { label: 'View', onClick: () => onView?.(item) },
            { label: 'Edit', onClick: () => onEdit?.(item) },
            { type: 'divider' },
            { label: 'Delete', onClick: () => onDelete?.(item), className: 'text-red-600' },
          ]}
        />
      ),
    },
  ];

  return <Table columns={columns} data={data} loading={loading} onRowClick={onView} />;
}
`, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, e.Name, columns, e.Name)
}

// =============================================================================
// DASHBOARD & AUTH
// =============================================================================

func generateDashboard(entities []Entity) string {
	cards := ""
	for _, e := range entities {
		lower := strings.ToLower(e.Name)
		icon := getIconForEntity(e.Name)
		cards += fmt.Sprintf(`      <StatCard title="Total %ss" value={stats.%s} icon="%s" />
`, e.Name, lower, icon)
	}

	statsInit := ""
	for _, e := range entities {
		lower := strings.ToLower(e.Name)
		statsInit += fmt.Sprintf("    %s: 0,\n", lower)
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { StatCard, Card, CardHeader } from '../components/ui/Card';

export default function DashboardPage() {
  const [stats, setStats] = useState({
%s  });

  useEffect(() => {
    // TODO: Fetch real stats from API
  }, []);

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Dashboard</h1>
        <p className="page-subtitle">Welcome back! Here's an overview of your data.</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
%s      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader title="Recent Activity" subtitle="Latest changes in the system" />
          <div className="text-gray-500 text-center py-8">No recent activity</div>
        </Card>
        
        <Card>
          <CardHeader title="Quick Actions" />
          <div className="space-y-2">
            <Link to="/users/new" className="block p-3 rounded-lg hover:bg-gray-50 border">
              + Create new user
            </Link>
          </div>
        </Card>
      </div>
    </div>
  );
}
`, statsInit, cards)
}

// =============================================================================
// AUTH PAGES
// =============================================================================

func generateLoginPage(features Features) string {
	oauthButtons := ""
	if features.HasAuthOAuth {
		oauthButtons = `
          <div className="relative my-6">
            <div className="absolute inset-0 flex items-center"><div className="w-full border-t border-gray-300" /></div>
            <div className="relative flex justify-center text-sm"><span className="px-2 bg-white text-gray-500">Or continue with</span></div>
          </div>
          
          <div className="grid grid-cols-2 gap-3">
            <button onClick={() => handleOAuth('google')} type="button" className="btn btn-secondary">
              <span className="mr-2">G</span> Google
            </button>
            <button onClick={() => handleOAuth('github')} type="button" className="btn btn-secondary">
              <span className="mr-2">🐙</span> GitHub
            </button>
          </div>`
	}

	oauthHandler := ""
	if features.HasAuthOAuth {
		oauthHandler = `
  const handleOAuth = async (provider: string) => {
    try {
      const res = await fetch(API_BASE + '/auth/oauth/' + provider + '/start');
      const { url } = await res.json();
      window.location.href = url;
    } catch (err: any) {
      setError(err.message);
    }
  };
`
	}

	forgotLink := ""
	if features.HasAuthEmail {
		forgotLink = `
            <div className="text-right">
              <Link to="/forgot-password" className="text-sm text-indigo-600 hover:underline">Forgot password?</Link>
            </div>`
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';

const API_BASE = import.meta.env.VITE_API_URL || '/api';

export default function LoginPage() {
  const navigate = useNavigate();
  const { login } = useAuth();
  const [form, setForm] = useState({ email: '', password: '' });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
%s
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      await login(form.email, form.password);
      navigate('/');
    } catch (err: any) {
      setError(err.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold">Welcome back</h1>
          <p className="text-gray-500 mt-2">Sign in to your account</p>
        </div>

        <div className="card p-8">
          {error && (
            <div className="mb-4 p-3 bg-red-50 text-red-700 rounded-lg text-sm">{error}</div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <Input label="Email" type="email" value={form.email} onChange={e => setForm(f => ({ ...f, email: e.target.value }))} required />
            <Input label="Password" type="password" value={form.password} onChange={e => setForm(f => ({ ...f, password: e.target.value }))} required />%s
            <Button type="submit" className="w-full" loading={loading}>Sign In</Button>
          </form>
%s
          <p className="mt-6 text-center text-sm text-gray-500">
            Don't have an account? <Link to="/signup" className="text-indigo-600 hover:underline">Sign up</Link>
          </p>
        </div>
      </div>
    </div>
  );
}
`, oauthHandler, forgotLink, oauthButtons)
}

func generateSignupPage(features Features) string {
	oauthButtons := ""
	if features.HasAuthOAuth {
		oauthButtons = `
          <div className="relative my-6">
            <div className="absolute inset-0 flex items-center"><div className="w-full border-t border-gray-300" /></div>
            <div className="relative flex justify-center text-sm"><span className="px-2 bg-white text-gray-500">Or continue with</span></div>
          </div>
          
          <div className="grid grid-cols-2 gap-3">
            <button onClick={() => handleOAuth('google')} type="button" className="btn btn-secondary">
              <span className="mr-2">G</span> Google
            </button>
            <button onClick={() => handleOAuth('github')} type="button" className="btn btn-secondary">
              <span className="mr-2">🐙</span> GitHub
            </button>
          </div>`
	}

	oauthHandler := ""
	if features.HasAuthOAuth {
		oauthHandler = `
  const handleOAuth = async (provider: string) => {
    try {
      const res = await fetch(API_BASE + '/auth/oauth/' + provider + '/start');
      const { url } = await res.json();
      window.location.href = url;
    } catch (err: any) {
      setError(err.message);
    }
  };
`
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';

const API_BASE = import.meta.env.VITE_API_URL || '/api';

export default function SignupPage() {
  const navigate = useNavigate();
  const { signup } = useAuth();
  const [form, setForm] = useState({ name: '', email: '', password: '', confirm: '' });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
%s
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (form.password !== form.confirm) { setError('Passwords do not match'); return; }
    setError('');
    setLoading(true);
    try {
      await signup(form.name, form.email, form.password);
      navigate('/');
    } catch (err: any) {
      setError(err.message || 'Signup failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold">Create account</h1>
          <p className="text-gray-500 mt-2">Get started with your free account</p>
        </div>

        <div className="card p-8">
          {error && <div className="mb-4 p-3 bg-red-50 text-red-700 rounded-lg text-sm">{error}</div>}

          <form onSubmit={handleSubmit} className="space-y-4">
            <Input label="Name" value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} required />
            <Input label="Email" type="email" value={form.email} onChange={e => setForm(f => ({ ...f, email: e.target.value }))} required />
            <Input label="Password" type="password" value={form.password} onChange={e => setForm(f => ({ ...f, password: e.target.value }))} required minLength={8} />
            <Input label="Confirm Password" type="password" value={form.confirm} onChange={e => setForm(f => ({ ...f, confirm: e.target.value }))} required />
            <Button type="submit" className="w-full" loading={loading}>Create Account</Button>
          </form>
%s
          <p className="mt-6 text-center text-sm text-gray-500">
            Already have an account? <Link to="/login" className="text-indigo-600 hover:underline">Sign in</Link>
          </p>
        </div>
      </div>
    </div>
  );
}
`, oauthHandler, oauthButtons)
}

func generateForgotPasswordPage() string {
	return `// Generated by protoc-gen-react-app
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';
import { authEmailApi } from '../api';

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('');
  const [sent, setSent] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      await authEmailApi.forgotPassword(email);
      setSent(true);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  if (sent) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
        <div className="card p-8 max-w-md w-full text-center">
          <div className="text-5xl mb-4">📧</div>
          <h1 className="text-2xl font-bold mb-2">Check your email</h1>
          <p className="text-gray-500 mb-6">We sent a password reset link to {email}</p>
          <Link to="/login" className="text-indigo-600 hover:underline">Back to login</Link>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold">Forgot password?</h1>
          <p className="text-gray-500 mt-2">Enter your email to reset your password</p>
        </div>

        <div className="card p-8">
          {error && <div className="mb-4 p-3 bg-red-50 text-red-700 rounded-lg text-sm">{error}</div>}
          <form onSubmit={handleSubmit} className="space-y-4">
            <Input label="Email" type="email" value={email} onChange={e => setEmail(e.target.value)} required />
            <Button type="submit" className="w-full" loading={loading}>Send Reset Link</Button>
          </form>
          <p className="mt-6 text-center text-sm text-gray-500">
            <Link to="/login" className="text-indigo-600 hover:underline">Back to login</Link>
          </p>
        </div>
      </div>
    </div>
  );
}
`
}

func generateResetPasswordPage() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';
import { Link, useSearchParams, useNavigate } from 'react-router-dom';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';
import { authEmailApi } from '../api';

export default function ResetPasswordPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const [form, setForm] = useState({ password: '', confirm: '' });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const token = searchParams.get('token');

  useEffect(() => { if (!token) setError('Invalid reset link'); }, [token]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (form.password !== form.confirm) { setError('Passwords do not match'); return; }
    if (!token) { setError('Invalid reset link'); return; }
    setError('');
    setLoading(true);
    try {
      await authEmailApi.resetPassword(token, form.password);
      navigate('/login?reset=success');
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold">Reset password</h1>
          <p className="text-gray-500 mt-2">Enter your new password</p>
        </div>
        <div className="card p-8">
          {error && <div className="mb-4 p-3 bg-red-50 text-red-700 rounded-lg text-sm">{error}</div>}
          <form onSubmit={handleSubmit} className="space-y-4">
            <Input label="New Password" type="password" value={form.password} onChange={e => setForm(f => ({ ...f, password: e.target.value }))} required minLength={8} />
            <Input label="Confirm Password" type="password" value={form.confirm} onChange={e => setForm(f => ({ ...f, confirm: e.target.value }))} required />
            <Button type="submit" className="w-full" loading={loading} disabled={!token}>Reset Password</Button>
          </form>
          <p className="mt-6 text-center text-sm text-gray-500">
            <Link to="/login" className="text-indigo-600 hover:underline">Back to login</Link>
          </p>
        </div>
      </div>
    </div>
  );
}
`
}

func generateVerifyEmailPage() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Loading } from '../components/ui/Loading';
import { authEmailApi } from '../api';

export default function VerifyEmailPage() {
  const [searchParams] = useSearchParams();
  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading');
  const [error, setError] = useState('');
  const token = searchParams.get('token');

  useEffect(() => {
    if (!token) { setStatus('error'); setError('Invalid verification link'); return; }
    authEmailApi.verifyEmail(token)
      .then(() => setStatus('success'))
      .catch(err => { setStatus('error'); setError(err.message); });
  }, [token]);

  if (status === 'loading') return <Loading fullScreen />;

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="card p-8 max-w-md w-full text-center">
        {status === 'success' ? (
          <>
            <div className="text-5xl mb-4">✅</div>
            <h1 className="text-2xl font-bold mb-2">Email verified!</h1>
            <p className="text-gray-500 mb-6">Your email has been successfully verified.</p>
          </>
        ) : (
          <>
            <div className="text-5xl mb-4">❌</div>
            <h1 className="text-2xl font-bold mb-2">Verification failed</h1>
            <p className="text-gray-500 mb-6">{error}</p>
          </>
        )}
        <Link to="/login" className="btn btn-primary">Go to Login</Link>
      </div>
    </div>
  );
}
`
}

func generateOAuthCallbackPage() string {
	return `// Generated by protoc-gen-react-app
import { useEffect, useState } from 'react';
import { useParams, useSearchParams, useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { Loading } from '../components/ui/Loading';
import { oauthApi } from '../api';

export default function OAuthCallbackPage() {
  const { provider } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { setAuth } = useAuth();
  const [error, setError] = useState('');

  useEffect(() => {
    const code = searchParams.get('code');
    const state = searchParams.get('state');
    if (!code || !state || !provider) { setError('Invalid OAuth callback'); return; }

    oauthApi.callback(provider, code, state)
      .then(({ token, user }) => {
        localStorage.setItem('token', token);
        localStorage.setItem('user', JSON.stringify(user));
        if (setAuth) setAuth({ token, user });
        navigate('/');
      })
      .catch(err => setError(err.message));
  }, [provider, searchParams, navigate, setAuth]);

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
        <div className="card p-8 max-w-md w-full text-center">
          <div className="text-5xl mb-4">❌</div>
          <h1 className="text-2xl font-bold mb-2">Authentication failed</h1>
          <p className="text-gray-500 mb-6">{error}</p>
          <a href="/login" className="btn btn-primary">Back to Login</a>
        </div>
      </div>
    );
  }
  return <Loading fullScreen />;
}
`
}

func generateLinkedAccountsPage() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { useToast } from '../components/ui/Toast';
import { oauthApi } from '../api';

const providers = [
  { id: 'google', name: 'Google', icon: 'G', color: 'bg-red-500' },
  { id: 'github', name: 'GitHub', icon: '🐙', color: 'bg-gray-800' },
  { id: 'apple', name: 'Apple', icon: '', color: 'bg-black' },
];

export default function LinkedAccountsPage() {
  const [linked, setLinked] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const { showToast } = useToast();

  useEffect(() => {
    oauthApi.getLinkedProviders()
      .then(({ providers }) => setLinked(providers))
      .finally(() => setLoading(false));
  }, []);

  const handleLink = async (provider: string) => {
    try {
      const { url } = await oauthApi.linkProvider(provider);
      window.location.href = url;
    } catch (err: any) {
      showToast(err.message, 'error');
    }
  };

  const handleUnlink = async (provider: string) => {
    try {
      await oauthApi.unlinkProvider(provider);
      setLinked(l => l.filter(p => p !== provider));
      showToast('Account unlinked', 'success');
    } catch (err: any) {
      showToast(err.message, 'error');
    }
  };

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Linked Accounts</h1>
        <p className="page-subtitle">Manage your connected social accounts</p>
      </div>
      <Card className="max-w-2xl">
        <div className="divide-y">
          {providers.map(p => {
            const isLinked = linked.includes(p.id);
            return (
              <div key={p.id} className="flex items-center justify-between p-4">
                <div className="flex items-center gap-3">
                  <div className={"w-10 h-10 rounded-full flex items-center justify-center text-white " + p.color}>{p.icon}</div>
                  <div>
                    <p className="font-medium">{p.name}</p>
                    <p className="text-sm text-gray-500">{isLinked ? 'Connected' : 'Not connected'}</p>
                  </div>
                </div>
                {isLinked ? (
                  <Button variant="danger" size="sm" onClick={() => handleUnlink(p.id)}>Unlink</Button>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => handleLink(p.id)}>Link</Button>
                )}
              </div>
            );
          })}
        </div>
      </Card>
    </div>
  );
}
`
}

func generatePricingPage() string {
	return `// Generated by protoc-gen-react-app
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '../components/ui/Button';
import { useAuth } from '../context/AuthContext';
import { stripeApi } from '../api';

const plans = [
  { id: 'free', name: 'Free', price: 0, priceId: '', features: ['5 projects', '1 GB storage', 'Community support'] },
  { id: 'pro', name: 'Pro', price: 29, priceId: 'price_pro_monthly', features: ['Unlimited projects', '100 GB storage', 'Priority support', 'Advanced analytics'], popular: true },
  { id: 'enterprise', name: 'Enterprise', price: 99, priceId: 'price_enterprise_monthly', features: ['Everything in Pro', 'Unlimited storage', 'Dedicated support', 'Custom integrations', 'SLA'] },
];

export default function PricingPage() {
  const navigate = useNavigate();
  const { isAuthenticated } = useAuth();
  const [loading, setLoading] = useState<string | null>(null);

  const handleSelect = async (plan: typeof plans[0]) => {
    if (!plan.priceId) return;
    if (!isAuthenticated) { navigate('/signup?plan=' + plan.id); return; }
    setLoading(plan.id);
    try {
      const { url } = await stripeApi.createCheckoutSession(plan.priceId);
      window.location.href = url;
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(null);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 py-16 px-4">
      <div className="max-w-5xl mx-auto">
        <div className="text-center mb-12">
          <h1 className="text-4xl font-bold">Simple, transparent pricing</h1>
          <p className="text-gray-500 mt-4 text-lg">Choose the plan that is right for you</p>
        </div>
        <div className="grid md:grid-cols-3 gap-8">
          {plans.map(plan => (
            <div key={plan.id} className={"card p-8 relative " + (plan.popular ? 'ring-2 ring-indigo-600' : '')}>
              {plan.popular && <span className="absolute -top-3 left-1/2 -translate-x-1/2 bg-indigo-600 text-white text-xs px-3 py-1 rounded-full">Most Popular</span>}
              <h2 className="text-xl font-bold">{plan.name}</h2>
              <div className="mt-4"><span className="text-4xl font-bold">${plan.price}</span><span className="text-gray-500">/month</span></div>
              <ul className="mt-6 space-y-3">
                {plan.features.map(f => <li key={f} className="flex items-center gap-2 text-sm"><span className="text-green-500">✓</span> {f}</li>)}
              </ul>
              <Button onClick={() => handleSelect(plan)} loading={loading === plan.id} variant={plan.popular ? 'primary' : 'secondary'} className="w-full mt-8">
                {plan.price === 0 ? 'Get Started' : 'Subscribe'}
              </Button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
`
}

func generateBillingPage() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';
import { Card, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';
import { Loading } from '../components/ui/Loading';
import { useToast } from '../components/ui/Toast';
import { stripeApi } from '../api';

export default function BillingPage() {
  const [subscription, setSubscription] = useState<any>(null);
  const [invoices, setInvoices] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const { showToast } = useToast();

  useEffect(() => {
    Promise.all([stripeApi.getSubscription(), stripeApi.getInvoices()])
      .then(([sub, inv]) => { setSubscription(sub); setInvoices(inv); })
      .finally(() => setLoading(false));
  }, []);

  const handleManage = async () => {
    const { url } = await stripeApi.createPortalSession();
    window.location.href = url;
  };

  const handleCancel = async () => {
    if (!confirm('Are you sure you want to cancel?')) return;
    try {
      await stripeApi.cancelSubscription();
      showToast('Subscription will be cancelled at period end', 'success');
      setSubscription((s: any) => ({ ...s, cancelAtPeriodEnd: true }));
    } catch (err: any) {
      showToast(err.message, 'error');
    }
  };

  const handleReactivate = async () => {
    try {
      await stripeApi.reactivateSubscription();
      showToast('Subscription reactivated', 'success');
      setSubscription((s: any) => ({ ...s, cancelAtPeriodEnd: false }));
    } catch (err: any) {
      showToast(err.message, 'error');
    }
  };

  if (loading) return <Loading />;

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Billing</h1>
        <p className="page-subtitle">Manage your subscription and billing</p>
      </div>
      <div className="grid gap-6 max-w-4xl">
        <Card>
          <CardHeader title="Current Plan" action={<Button variant="secondary" onClick={handleManage}>Manage Billing</Button>} />
          {subscription ? (
            <div className="space-y-4">
              <div className="flex items-center gap-3">
                <Badge variant={subscription.status === 'active' ? 'success' : 'warning'}>{subscription.status}</Badge>
                {subscription.cancelAtPeriodEnd && <Badge variant="warning">Cancels at period end</Badge>}
              </div>
              {subscription.currentPeriodEnd && <p className="text-gray-600">Next billing: {new Date(subscription.currentPeriodEnd).toLocaleDateString()}</p>}
              <div className="flex gap-2">
                {subscription.cancelAtPeriodEnd ? (
                  <Button onClick={handleReactivate}>Reactivate</Button>
                ) : (
                  <Button variant="danger" onClick={handleCancel}>Cancel Subscription</Button>
                )}
              </div>
            </div>
          ) : (
            <div className="text-center py-8">
              <p className="text-gray-500 mb-4">No active subscription</p>
              <Button onClick={() => window.location.href = '/pricing'}>View Plans</Button>
            </div>
          )}
        </Card>
        <Card>
          <CardHeader title="Billing History" />
          {invoices.length === 0 ? <p className="text-gray-500 text-center py-4">No invoices yet</p> : (
            <table className="w-full">
              <thead className="bg-gray-50"><tr>
                <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Date</th>
                <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Amount</th>
                <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Status</th>
                <th className="px-4 py-2 text-right text-sm font-medium text-gray-500">Invoice</th>
              </tr></thead>
              <tbody className="divide-y">
                {invoices.map((inv: any) => (
                  <tr key={inv.id}>
                    <td className="px-4 py-3 text-sm">{new Date(inv.created * 1000).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-sm">${(inv.amount_paid / 100).toFixed(2)}</td>
                    <td className="px-4 py-3"><Badge variant={inv.status === 'paid' ? 'success' : 'warning'}>{inv.status}</Badge></td>
                    <td className="px-4 py-3 text-right"><a href={inv.invoice_pdf} className="text-indigo-600 hover:underline text-sm">Download</a></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      </div>
    </div>
  );
}
`
}

func generateCheckoutSuccessPage() string {
	return `// Generated by protoc-gen-react-app
import { Link } from 'react-router-dom';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';

export default function CheckoutSuccessPage() {
  return (
    <div className="max-w-md mx-auto mt-16">
      <Card className="text-center p-8">
        <div className="text-6xl mb-4">🎉</div>
        <h1 className="text-2xl font-bold mb-2">Thank you!</h1>
        <p className="text-gray-500 mb-6">Your subscription is now active. You can start using all premium features.</p>
        <div className="flex gap-3 justify-center">
          <Link to="/"><Button>Go to Dashboard</Button></Link>
          <Link to="/settings/billing"><Button variant="secondary">View Billing</Button></Link>
        </div>
      </Card>
    </div>
  );
}
`
}

func generateNotificationsPage() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Loading } from '../components/ui/Loading';
import { EmptyState } from '../components/ui/EmptyState';
import { notificationApi } from '../api';

export default function NotificationsPage() {
  const [notifications, setNotifications] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    notificationApi.list({ limit: 50 }).then(setNotifications).finally(() => setLoading(false));
  }, []);

  const handleMarkAsRead = async (id: string) => {
    await notificationApi.markAsRead(id);
    setNotifications(n => n.map(x => x.id === id ? { ...x, readAt: new Date().toISOString() } : x));
  };

  const handleMarkAllAsRead = async () => {
    await notificationApi.markAllAsRead();
    setNotifications(n => n.map(x => ({ ...x, readAt: new Date().toISOString() })));
  };

  if (loading) return <Loading />;

  return (
    <div>
      <div className="page-header flex items-center justify-between">
        <div>
          <h1 className="page-title">Notifications</h1>
          <p className="page-subtitle">{notifications.filter(n => !n.readAt).length} unread</p>
        </div>
        <Button variant="secondary" onClick={handleMarkAllAsRead}>Mark all as read</Button>
      </div>
      {notifications.length === 0 ? (
        <EmptyState icon="🔔" title="No notifications" description="You are all caught up!" />
      ) : (
        <Card padding={false}>
          <div className="divide-y">
            {notifications.map(n => (
              <div key={n.id} onClick={() => !n.readAt && handleMarkAsRead(n.id)} className={"p-4 cursor-pointer hover:bg-gray-50 " + (!n.readAt ? 'bg-blue-50' : '')}>
                <div className="flex justify-between items-start">
                  <div>
                    <p className="font-medium">{n.title}</p>
                    <p className="text-sm text-gray-600 mt-1">{n.body}</p>
                  </div>
                  <span className="text-xs text-gray-400">{new Date(n.createdAt).toLocaleString()}</span>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}
`
}

func generateNotificationSettingsPage() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';
import { Card, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Input';
import { Loading } from '../components/ui/Loading';
import { useToast } from '../components/ui/Toast';
import { notificationApi } from '../api';

export default function NotificationSettingsPage() {
  const [prefs, setPrefs] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const { showToast } = useToast();

  useEffect(() => { notificationApi.getPreferences().then(setPrefs).finally(() => setLoading(false)); }, []);

  const handleSave = async () => {
    setSaving(true);
    try { await notificationApi.updatePreferences(prefs); showToast('Preferences saved', 'success'); }
    catch (err: any) { showToast(err.message, 'error'); }
    finally { setSaving(false); }
  };

  const handleTest = async () => {
    try { await notificationApi.testNotification(); showToast('Test notification sent', 'success'); }
    catch (err: any) { showToast(err.message, 'error'); }
  };

  if (loading) return <Loading />;

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Notification Settings</h1>
        <p className="page-subtitle">Manage how you receive notifications</p>
      </div>
      <div className="max-w-2xl space-y-6">
        <Card><CardHeader title="Push Notifications" /><Checkbox label="Enable push notifications" checked={prefs?.pushEnabled} onChange={e => setPrefs((p: any) => ({ ...p, pushEnabled: e.target.checked }))} /></Card>
        <Card><CardHeader title="Email Notifications" /><Checkbox label="Enable email notifications" checked={prefs?.emailEnabled} onChange={e => setPrefs((p: any) => ({ ...p, emailEnabled: e.target.checked }))} /></Card>
        <Card><CardHeader title="SMS Notifications" /><Checkbox label="Enable SMS notifications" checked={prefs?.smsEnabled} onChange={e => setPrefs((p: any) => ({ ...p, smsEnabled: e.target.checked }))} /></Card>
        <Card>
          <CardHeader title="Quiet Hours" subtitle="Do not send notifications during these hours" />
          <div className="flex gap-4">
            <div className="flex-1"><label className="label">Start</label><input type="time" className="input" value={prefs?.quietHoursStart || ''} onChange={e => setPrefs((p: any) => ({ ...p, quietHoursStart: e.target.value }))} /></div>
            <div className="flex-1"><label className="label">End</label><input type="time" className="input" value={prefs?.quietHoursEnd || ''} onChange={e => setPrefs((p: any) => ({ ...p, quietHoursEnd: e.target.value }))} /></div>
          </div>
        </Card>
        <div className="flex gap-3">
          <Button onClick={handleSave} loading={saving}>Save Preferences</Button>
          <Button variant="secondary" onClick={handleTest}>Send Test Notification</Button>
        </div>
      </div>
    </div>
  );
}
`
}

func generateNotificationBellComponent() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect, useRef } from 'react';
import { Link } from 'react-router-dom';
import { notificationApi } from '../api';

export function NotificationBell() {
  const [notifications, setNotifications] = useState<any[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => { notificationApi.getUnreadCount().then(({ count }) => setUnreadCount(count)); }, []);
  useEffect(() => { if (open) notificationApi.list({ limit: 5 }).then(setNotifications); }, [open]);
  useEffect(() => {
    const handleClick = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false); };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  const handleMarkAsRead = async (id: string) => {
    await notificationApi.markAsRead(id);
    setNotifications(n => n.map(x => x.id === id ? { ...x, readAt: new Date().toISOString() } : x));
    setUnreadCount(c => Math.max(0, c - 1));
  };

  return (
    <div ref={ref} className="relative">
      <button onClick={() => setOpen(!open)} className="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg relative">
        🔔
        {unreadCount > 0 && <span className="absolute -top-1 -right-1 w-5 h-5 bg-red-500 text-white text-xs rounded-full flex items-center justify-center">{unreadCount > 9 ? '9+' : unreadCount}</span>}
      </button>
      {open && (
        <div className="absolute right-0 mt-2 w-80 bg-white rounded-lg shadow-lg border z-50">
          <div className="p-3 border-b flex justify-between items-center">
            <span className="font-medium">Notifications</span>
            <Link to="/notifications" className="text-sm text-indigo-600" onClick={() => setOpen(false)}>View all</Link>
          </div>
          <div className="max-h-96 overflow-y-auto">
            {notifications.length === 0 ? <div className="p-4 text-center text-gray-500">No notifications</div> : notifications.map(n => (
              <div key={n.id} onClick={() => !n.readAt && handleMarkAsRead(n.id)} className={"p-3 border-b cursor-pointer hover:bg-gray-50 " + (!n.readAt ? "bg-blue-50" : "")}>
                <p className="font-medium text-sm">{n.title}</p>
                <p className="text-xs text-gray-600 mt-1">{n.body}</p>
                <p className="text-xs text-gray-400 mt-1">{new Date(n.createdAt).toLocaleString()}</p>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
`
}

// Geo Pages
func generateStoreLocatorPage() string {
	return `// Generated by protoc-gen-react-app
import { useState, useEffect } from 'react';
import { Card } from '../components/ui/Card';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';
import { Loading } from '../components/ui/Loading';
import { MapView } from '../components/MapView';
import { geoApi } from '../api';

export default function StoreLocatorPage() {
  const [locations, setLocations] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [userLocation, setUserLocation] = useState<{ lat: number; lng: number } | null>(null);
  const [selectedLocation, setSelectedLocation] = useState<any>(null);

  useEffect(() => {
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        const loc = { lat: pos.coords.latitude, lng: pos.coords.longitude };
        setUserLocation(loc);
        fetchNearby(loc.lat, loc.lng);
      },
      () => fetchNearby(40.7128, -74.006) // Default to NYC
    );
  }, []);

  const fetchNearby = async (lat: number, lng: number) => {
    setLoading(true);
    try {
      const results = await geoApi.findNearest(lat, lng, 50, 20);
      setLocations(results);
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = async () => {
    if (!search.trim()) return;
    setLoading(true);
    try {
      const results = await geoApi.search(search);
      setLocations(results);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 py-8">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold">Find a Location</h1>
          <p className="text-gray-500 mt-2">Search for stores near you</p>
        </div>

        <div className="flex gap-4 mb-6 max-w-xl mx-auto">
          <Input placeholder="Enter city, zip, or address" value={search} onChange={e => setSearch(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleSearch()} />
          <Button onClick={handleSearch}>Search</Button>
          {userLocation && <Button variant="secondary" onClick={() => fetchNearby(userLocation.lat, userLocation.lng)}>📍 Near Me</Button>}
        </div>

        <div className="grid lg:grid-cols-3 gap-6">
          <div className="lg:col-span-2 h-[500px] rounded-xl overflow-hidden">
            <MapView locations={locations} center={userLocation} onSelect={setSelectedLocation} selected={selectedLocation} />
          </div>
          
          <div className="space-y-4 max-h-[500px] overflow-y-auto">
            {loading ? <Loading /> : locations.length === 0 ? (
              <Card className="text-center py-8 text-gray-500">No locations found</Card>
            ) : locations.map((loc, i) => (
              <Card key={loc.id || i} className={"cursor-pointer transition-shadow hover:shadow-md " + (selectedLocation?.id === loc.id ? 'ring-2 ring-indigo-500' : '')} onClick={() => setSelectedLocation(loc)}>
                <h3 className="font-medium">{loc.name}</h3>
                <p className="text-sm text-gray-500 mt-1">{loc.address}</p>
                {loc.distance && <p className="text-sm text-indigo-600 mt-2">{loc.distance.toFixed(1)} km away</p>}
                {loc.phone && <p className="text-sm text-gray-500 mt-1">📞 {loc.phone}</p>}
              </Card>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
`
}

func generateMapViewComponent() string {
	return `// Generated by protoc-gen-react-app
import { useEffect, useRef, useState } from 'react';

interface Location {
  id: string;
  name: string;
  latitude: number;
  longitude: number;
  address?: string;
}

interface MapViewProps {
  locations: Location[];
  center?: { lat: number; lng: number } | null;
  onSelect?: (location: Location) => void;
  selected?: Location | null;
}

export function MapView({ locations, center, onSelect, selected }: MapViewProps) {
  const mapRef = useRef<HTMLDivElement>(null);
  const [map, setMap] = useState<google.maps.Map | null>(null);
  const [markers, setMarkers] = useState<google.maps.Marker[]>([]);

  useEffect(() => {
    if (!mapRef.current || map) return;
    
    const newMap = new google.maps.Map(mapRef.current, {
      center: center || { lat: 40.7128, lng: -74.006 },
      zoom: 12,
      styles: [{ featureType: 'poi', elementType: 'labels', stylers: [{ visibility: 'off' }] }],
    });
    setMap(newMap);
  }, []);

  useEffect(() => {
    if (!map) return;
    
    // Clear existing markers
    markers.forEach(m => m.setMap(null));
    
    // Add new markers
    const newMarkers = locations.map(loc => {
      const marker = new google.maps.Marker({
        position: { lat: loc.latitude, lng: loc.longitude },
        map,
        title: loc.name,
        icon: selected?.id === loc.id ? 'https://maps.google.com/mapfiles/ms/icons/blue-dot.png' : undefined,
      });
      marker.addListener('click', () => onSelect?.(loc));
      return marker;
    });
    
    setMarkers(newMarkers);
    
    // Fit bounds
    if (locations.length > 0) {
      const bounds = new google.maps.LatLngBounds();
      locations.forEach(loc => bounds.extend({ lat: loc.latitude, lng: loc.longitude }));
      map.fitBounds(bounds);
    }
  }, [map, locations, selected]);

  useEffect(() => {
    if (map && center) {
      map.setCenter(center);
    }
  }, [map, center]);

  return (
    <>
      <div ref={mapRef} className="w-full h-full" />
      {/* Fallback if Google Maps not loaded */}
      <script src={"https://maps.googleapis.com/maps/api/js?key=" + (import.meta.env.VITE_GOOGLE_MAPS_KEY || '')} />
    </>
  );
}
`
}

// Settings Pages
func generateSettingsPage(features Features) string {
	sections := ""
	if features.HasStripe {
		sections += `
        <Link to="/settings/billing" className="flex items-center gap-4 p-4 rounded-lg hover:bg-gray-50 border">
          <span className="text-2xl">💳</span>
          <div>
            <p className="font-medium">Billing</p>
            <p className="text-sm text-gray-500">Manage subscription and payment methods</p>
          </div>
        </Link>`
	}
	if features.HasNotification {
		sections += `
        <Link to="/settings/notifications" className="flex items-center gap-4 p-4 rounded-lg hover:bg-gray-50 border">
          <span className="text-2xl">🔔</span>
          <div>
            <p className="font-medium">Notifications</p>
            <p className="text-sm text-gray-500">Configure how you receive notifications</p>
          </div>
        </Link>`
	}
	if features.HasAuthOAuth {
		sections += `
        <Link to="/settings/linked-accounts" className="flex items-center gap-4 p-4 rounded-lg hover:bg-gray-50 border">
          <span className="text-2xl">🔗</span>
          <div>
            <p className="font-medium">Linked Accounts</p>
            <p className="text-sm text-gray-500">Manage connected social accounts</p>
          </div>
        </Link>`
	}

	return fmt.Sprintf(`// Generated by protoc-gen-react-app
import { Link } from 'react-router-dom';
import { Card, CardHeader } from '../components/ui/Card';

export default function SettingsPage() {
  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Settings</h1>
        <p className="page-subtitle">Manage your account settings</p>
      </div>

      <div className="max-w-2xl space-y-4">
        <Link to="/profile" className="flex items-center gap-4 p-4 rounded-lg hover:bg-gray-50 border bg-white">
          <span className="text-2xl">👤</span>
          <div>
            <p className="font-medium">Profile</p>
            <p className="text-sm text-gray-500">Update your personal information</p>
          </div>
        </Link>
        
        <Link to="/profile" className="flex items-center gap-4 p-4 rounded-lg hover:bg-gray-50 border bg-white">
          <span className="text-2xl">🔒</span>
          <div>
            <p className="font-medium">Security</p>
            <p className="text-sm text-gray-500">Change password and security settings</p>
          </div>
        </Link>
%s
      </div>
    </div>
  );
}
`, sections)
}

func generateProfilePage() string {
	return `// Generated by protoc-gen-react-app
import { useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { Card, CardHeader } from '../components/ui/Card';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';
import { useToast } from '../components/ui/Toast';

export default function ProfilePage() {
  const { user } = useAuth();
  const { showToast } = useToast();
  const [form, setForm] = useState({ name: user?.name || '', email: user?.email || '' });
  const [passwordForm, setPasswordForm] = useState({ current: '', newPassword: '', confirm: '' });
  const [loading, setLoading] = useState(false);

  const handleUpdateProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      // await authApi.updateProfile(form);
      showToast('Profile updated', 'success');
    } catch (err: any) {
      showToast(err.message, 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (passwordForm.newPassword !== passwordForm.confirm) {
      showToast('Passwords do not match', 'error');
      return;
    }
    setLoading(true);
    try {
      // await authApi.changePassword(passwordForm);
      showToast('Password changed', 'success');
      setPasswordForm({ current: '', newPassword: '', confirm: '' });
    } catch (err: any) {
      showToast(err.message, 'error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Profile</h1>
        <p className="page-subtitle">Manage your account information</p>
      </div>

      <div className="max-w-2xl space-y-6">
        <Card>
          <CardHeader title="Personal Information" />
          <form onSubmit={handleUpdateProfile} className="space-y-4">
            <Input label="Name" value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} />
            <Input label="Email" type="email" value={form.email} onChange={e => setForm(f => ({ ...f, email: e.target.value }))} />
            <Button type="submit" loading={loading}>Save Changes</Button>
          </form>
        </Card>

        <Card>
          <CardHeader title="Change Password" />
          <form onSubmit={handleChangePassword} className="space-y-4">
            <Input label="Current Password" type="password" value={passwordForm.current} onChange={e => setPasswordForm(f => ({ ...f, current: e.target.value }))} />
            <Input label="New Password" type="password" value={passwordForm.newPassword} onChange={e => setPasswordForm(f => ({ ...f, newPassword: e.target.value }))} />
            <Input label="Confirm New Password" type="password" value={passwordForm.confirm} onChange={e => setPasswordForm(f => ({ ...f, confirm: e.target.value }))} />
            <Button type="submit" loading={loading}>Change Password</Button>
          </form>
        </Card>

        <Card>
          <CardHeader title="Danger Zone" />
          <p className="text-gray-500 text-sm mb-4">Once you delete your account, there is no going back.</p>
          <Button variant="danger">Delete Account</Button>
        </Card>
      </div>
    </div>
  );
}
`
}

func generateAuthContext() string {
	return `// Generated by protoc-gen-react-app
import { createContext, useContext, useState, useEffect, useCallback } from 'react';

interface User {
  id: string;
  name: string;
  email: string;
}

interface AuthContextType {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  signup: (name: string, email: string, password: string) => Promise<void>;
  logout: () => void;
  setAuth?: (data: { token: string; user: User }) => void;
}

const AuthContext = createContext<AuthContextType | null>(null);

const API_BASE = import.meta.env.VITE_API_URL || '/api';

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const token = localStorage.getItem('token');
    const storedUser = localStorage.getItem('user');
    if (token && storedUser) {
      setUser(JSON.parse(storedUser));
    }
    setIsLoading(false);
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const res = await fetch(API_BASE + '/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });
    if (!res.ok) {
      const error = await res.json().catch(() => ({}));
      throw new Error(error.message || 'Login failed');
    }
    const data = await res.json();
    localStorage.setItem('token', data.token);
    localStorage.setItem('user', JSON.stringify(data.user));
    setUser(data.user);
  }, []);

  const signup = useCallback(async (name: string, email: string, password: string) => {
    const res = await fetch(API_BASE + '/auth/signup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, email, password }),
    });
    if (!res.ok) {
      const error = await res.json().catch(() => ({}));
      throw new Error(error.message || 'Signup failed');
    }
    const data = await res.json();
    localStorage.setItem('token', data.token);
    localStorage.setItem('user', JSON.stringify(data.user));
    setUser(data.user);
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    setUser(null);
  }, []);

  const setAuth = useCallback((data: { token: string; user: User }) => {
    localStorage.setItem('token', data.token);
    localStorage.setItem('user', JSON.stringify(data.user));
    setUser(data.user);
  }, []);

  return (
    <AuthContext.Provider value={{ user, isAuthenticated: !!user, isLoading, login, signup, logout, setAuth }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth requires AuthProvider');
  return ctx;
}
`
}

// =============================================================================
// CONFIG FILES
// =============================================================================

func generatePackageJson(features Features) string {
	deps := `    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "react-router-dom": "^6.20.0"`

	if features.HasStripe {
		deps += `,
    "@stripe/stripe-js": "^2.2.0"`
	}
	if features.HasGeo {
		deps += `,
    "@react-google-maps/api": "^2.19.0"`
	}

	return fmt.Sprintf(`{
  "name": "admin-app",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
%s
  },
  "devDependencies": {
    "@types/react": "^18.2.37",
    "@types/react-dom": "^18.2.15",
    "@vitejs/plugin-react": "^4.2.0",
    "autoprefixer": "^10.4.16",
    "postcss": "^8.4.31",
    "tailwindcss": "^3.3.5",
    "typescript": "^5.2.2",
    "vite": "^5.0.0"
  }
}
`, deps)
}

func generateViteConfig() string {
	return `import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});
`
}

func generateTailwindConfig() string {
	return `/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {},
  },
  plugins: [],
};
`
}

func generatePostcssConfig() string {
	return `export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
`
}

func generateTsConfig() string {
	return `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
`
}

func generateIndexHtml() string {
	return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/svg+xml" href="/vite.svg" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Admin Dashboard</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
`
}
