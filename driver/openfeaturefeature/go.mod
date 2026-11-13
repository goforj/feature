module github.com/goforj/feature/driver/openfeaturefeature

go 1.26.0

require (
	github.com/goforj/feature v0.0.0-20261113224422-3e7b49d5eec5
	github.com/open-feature/go-sdk v1.20.0
)

require go.uber.org/mock v0.6.0 // indirect

replace github.com/goforj/feature => ../..
