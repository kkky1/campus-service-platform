// Package buildinfo 构建信息（通过 -ldflags 在编译期注入）。
package buildinfo

// 默认值；CI/本地构建时用
//   -X campus-service-platform/internal/pkg/buildinfo.Version=$(git rev-parse --short HEAD)
//   -X campus-service-platform/internal/pkg/buildinfo.Commit=...
//   -X campus-service-platform/internal/pkg/buildinfo.Time=...
// 覆盖。
var (
	Version = "dev"
	Commit  = "unknown"
	Time    = "unknown"
)
