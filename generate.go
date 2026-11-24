package client

// Each product's package is generated from its public spec, which the
// platform's publicspec writes into spec/.
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 -generate types,client -package billing -o billing/billing.gen.go spec/billing.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 -generate types,client -package otp -o otp/otp.gen.go spec/otp.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 -generate types,client -package auth -o auth/auth.gen.go spec/auth.yaml
