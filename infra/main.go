package main

import (
	"log"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/config"
	"snap_kakeibo/infra/stacks"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	cfg, err := config.Load(contextString(app, "stage"))
	if err != nil {
		log.Fatalf("%v: pass -c stage=<name> or set it in cdk.json", err)
	}
	// 初回切替では既定 URL を残し、CloudFront の新オリジンの動作確認後に解除する。
	if contextString(app, "keepExecuteApiEndpoint") == "true" {
		cfg.Domains.DisableExecuteAPIEndpoint = false
	}
	if contextString(app, "keepExecuteApiOrigin") == "true" {
		cfg.Domains.UseLegacyAPIOrigin = true
		cfg.Domains.DisableExecuteAPIEndpoint = false
	}
	props := stackProps(cfg)
	certificateProps := awscdk.StackProps{
		Env:                   &awscdk.Environment{Account: jsii.String(cfg.AccountID), Region: jsii.String("us-east-1")},
		CrossRegionReferences: jsii.Bool(true),
	}
	certificate := stacks.NewFrontendCertificateStack(app, string(cfg.Stage)+"-snap-kakeibo-certificate", &certificateProps, cfg)

	storage := stacks.NewStorageStack(app, cfg.StorageStackName, &stacks.StorageStackProps{
		StackProps: props,
		Config:     cfg,
	})

	appProps := props
	appProps.CrossRegionReferences = jsii.Bool(true)
	stacks.NewAppStack(app, cfg.AppStackName, &stacks.AppStackProps{
		StackProps:          appProps,
		Config:              cfg,
		Storage:             storage,
		FrontendCertificate: certificate.Certificate,
	})

	stacks.NewPipelineStack(app, cfg.PipelineStackName, &stacks.PipelineStackProps{
		StackProps: props,
		Config:     cfg,
	})

	app.Synth(nil)
}

// contextString は -c key=value または cdk.json の context から文字列を読む。
func contextString(app awscdk.App, key string) string {
	if v, ok := app.Node().TryGetContext(jsii.String(key)).(string); ok {
		return v
	}
	return ""
}

func stackProps(cfg config.Config) awscdk.StackProps {
	return awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String(cfg.AccountID),
			Region:  jsii.String(cfg.Region),
		},
	}
}
