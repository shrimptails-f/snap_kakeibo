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
	props := stackProps(cfg)

	storage := stacks.NewStorageStack(app, cfg.StorageStackName, &stacks.StorageStackProps{
		StackProps: props,
		Config:     cfg,
	})

	stacks.NewAppStack(app, cfg.AppStackName, &stacks.AppStackProps{
		StackProps: props,
		Config:     cfg,
		Storage:    storage,
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
