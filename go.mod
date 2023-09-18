module github.com/longhorn/longhorn-preflight

go 1.20

require (
	github.com/longhorn/go-common-libs v0.0.0-20230725131218-5fe3b8fdf5d5
	github.com/otiai10/copy v1.12.0
	github.com/sirupsen/logrus v1.9.3
	github.com/urfave/cli v1.22.14
)

require (
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil/v3 v3.23.7 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	golang.org/x/sys v0.11.0 // indirect
)

replace github.com/longhorn/go-common-libs v0.0.0-20230725131218-5fe3b8fdf5d5 => github.com/c3y1huang/go-common-libs v0.0.0-20230908015436-886e1f60245c
