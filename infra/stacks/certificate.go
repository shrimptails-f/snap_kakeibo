package stacks

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/config"
)

// FrontendCertificateStack は CloudFront 専用の us-east-1 証明書を管理する。
type FrontendCertificateStack struct {
	awscdk.Stack
	Certificate awscertificatemanager.Certificate
}

func NewFrontendCertificateStack(scope constructs.Construct, id string, props *awscdk.StackProps, cfg config.Config) *FrontendCertificateStack {
	stack := awscdk.NewStack(scope, jsii.String(id), props)
	zone := awsroute53.HostedZone_FromHostedZoneAttributes(stack, jsii.String("SharedHostedZone"), &awsroute53.HostedZoneAttributes{
		HostedZoneId: jsii.String(cfg.Domains.HostedZoneID),
		ZoneName:     jsii.String(cfg.Domains.ZoneName),
	})
	certificate := awscertificatemanager.NewCertificate(stack, jsii.String("FrontendCertificate"), &awscertificatemanager.CertificateProps{
		DomainName: jsii.String(cfg.Domains.FrontendFQDN()),
		Validation: awscertificatemanager.CertificateValidation_FromDns(zone),
	})
	return &FrontendCertificateStack{Stack: stack, Certificate: certificate}
}
