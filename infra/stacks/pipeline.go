package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscodebuild"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscodepipeline"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscodepipelineactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/common"
	"snap_kakeibo/infra/config"
)

type PipelineStackProps struct {
	awscdk.StackProps
	Config config.Config
}

type PipelineStack struct {
	awscdk.Stack
}

func NewPipelineStack(scope constructs.Construct, id string, props *PipelineStackProps) *PipelineStack {
	stack := awscdk.NewStack(scope, jsii.String(id), &props.StackProps)
	s := &PipelineStack{Stack: stack}

	connectionARN := awsssm.StringParameter_ValueForStringParameter(stack, jsii.String(props.Config.CI.Parameters.GitHubConnectionARN), nil)
	awscdk.NewCfnRule(stack, jsii.String("GitHubConnectionArnIsSet"), &awscdk.CfnRuleProps{
		Assertions: &[]*awscdk.CfnRuleAssertion{{
			Assert:            awscdk.Fn_ConditionNot(awscdk.Fn_ConditionEquals(connectionARN, jsii.String("UNSET"))),
			AssertDescription: jsii.String(fmt.Sprintf("%s must be set before deploying this stack", props.Config.CI.Parameters.GitHubConnectionARN)),
		}},
	})

	s.newServicePipeline("backend", props.Config, connectionARN, backendBuildAndDeploySpec())
	s.newServicePipeline("frontend", props.Config, connectionARN, frontendBuildAndDeploySpec())
	return s
}

func (s *PipelineStack) newServicePipeline(name string, cfg config.Config, connectionARN *string, buildSpec awscodebuild.BuildSpec) {
	source := awscodepipeline.NewArtifact(jsii.String(constructID(name)+"Source"), nil)
	project := awscodebuild.NewPipelineProject(s.Stack, jsii.String(constructID(name)+"BuildAndDeployProject"), &awscodebuild.PipelineProjectProps{
		ProjectName: jsii.String(cfg.ResourceName(name + "-build-and-deploy")),
		Environment: &awscodebuild.BuildEnvironment{
			BuildImage:  awscodebuild.LinuxBuildImage_STANDARD_7_0(),
			ComputeType: awscodebuild.ComputeType_SMALL,
			Privileged:  jsii.Bool(name == "backend"),
		},
		BuildSpec: buildSpec,
		EnvironmentVariables: &map[string]*awscodebuild.BuildEnvironmentVariable{
			"STAGE":   {Value: jsii.String(string(cfg.Stage))},
			"PROJECT": {Value: jsii.String(common.ProjectResourceName)},
		},
	})
	grantCDProject(project, cfg, name, connectionARN)

	pipeline := awscodepipeline.NewPipeline(s.Stack, jsii.String(constructID(name)+"Pipeline"), &awscodepipeline.PipelineProps{
		PipelineName:             jsii.String(cfg.ResourceName(name)),
		RestartExecutionOnUpdate: jsii.Bool(true),
	})
	pipeline.AddStage(&awscodepipeline.StageOptions{
		StageName: jsii.String("Source"),
		Actions: &[]awscodepipeline.IAction{
			awscodepipelineactions.NewCodeStarConnectionsSourceAction(&awscodepipelineactions.CodeStarConnectionsSourceActionProps{
				ActionName:           jsii.String("GitHub"),
				Owner:                jsii.String(cfg.CI.GitHubOwner),
				Repo:                 jsii.String(cfg.CI.GitHubRepo),
				Branch:               jsii.String(cfg.CI.Branch),
				ConnectionArn:        connectionARN,
				CodeBuildCloneOutput: jsii.Bool(true),
				Output:               source,
			}),
		},
	})
	pipeline.AddStage(&awscodepipeline.StageOptions{
		StageName: jsii.String("BuildAndDeploy"),
		Actions: &[]awscodepipeline.IAction{
			awscodepipelineactions.NewCodeBuildAction(&awscodepipelineactions.CodeBuildActionProps{
				ActionName: jsii.String("BuildAndDeploy"),
				Project:    project,
				Input:      source,
			}),
		},
	})
}

func backendBuildAndDeploySpec() awscodebuild.BuildSpec {
	return awscodebuild.BuildSpec_FromObject(&map[string]any{
		"version": "0.2",
		"phases": map[string]any{
			"install": map[string]any{
				"runtime-versions": map[string]any{"golang": "latest"},
			},
			"pre_build": map[string]any{
				"commands": []string{
					"chmod +x scripts/*.sh",
					"aws --version",
					"docker --version",
				},
			},
			"build": map[string]any{
				"commands": []string{
					"bash scripts/backend-build.sh",
					"bash build/scripts/backend-deploy.sh build/backend-plan.json",
				},
			},
		},
	})
}

func frontendBuildAndDeploySpec() awscodebuild.BuildSpec {
	return awscodebuild.BuildSpec_FromObject(&map[string]any{
		"version": "0.2",
		"phases": map[string]any{
			"install": map[string]any{
				"runtime-versions": map[string]any{"nodejs": "latest"},
			},
			"pre_build": map[string]any{
				"commands": []string{"chmod +x scripts/*.sh", "node --version", "npm --version"},
			},
			"build": map[string]any{
				"commands": []string{
					"bash scripts/frontend-build.sh",
					"bash build/scripts/frontend-deploy.sh build/frontend-plan.json",
				},
			},
		},
	})
}

func grantCDProject(project awscodebuild.PipelineProject, cfg config.Config, name string, connectionARN *string) {
	project.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions:   jsii.Strings("ssm:GetParameter", "ssm:PutParameter"),
		Resources: jsii.Strings(fmt.Sprintf("arn:aws:ssm:%s:%s:parameter/%s/%s/*", cfg.Region, cfg.AccountID, cfg.Stage, common.ProjectResourceName)),
	}))
	project.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions:   jsii.Strings("cloudformation:DescribeStacks", "sts:GetCallerIdentity"),
		Resources: jsii.Strings("*"),
	}))
	project.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: jsii.Strings(
			"codestar-connections:UseConnection",
			"codeconnections:UseConnection",
			"codestar-connections:GetConnection",
			"codeconnections:GetConnection",
		),
		Resources: jsii.Strings(*connectionARN),
	}))
	if name == "backend" {
		project.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Actions: jsii.Strings(
				"ecr:BatchCheckLayerAvailability", "ecr:BatchGetImage", "ecr:CompleteLayerUpload", "ecr:DescribeImages",
				"ecr:GetAuthorizationToken", "ecr:InitiateLayerUpload", "ecr:PutImage", "ecr:UploadLayerPart",
				"lambda:GetAlias", "lambda:GetFunction", "lambda:UpdateFunctionCode",
				"codedeploy:CreateDeployment", "codedeploy:GetDeployment", "codedeploy:GetDeploymentConfig",
				"codedeploy:RegisterApplicationRevision",
			),
			Resources: jsii.Strings("*"),
		}))
		return
	}
	project.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions:   jsii.Strings("s3:ListBucket"),
		Resources: jsii.Strings(fmt.Sprintf("arn:aws:s3:::%s", cfg.Buckets.Frontend)),
	}))
	project.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions:   jsii.Strings("s3:DeleteObject", "s3:GetObject", "s3:PutObject"),
		Resources: jsii.Strings(fmt.Sprintf("arn:aws:s3:::%s/*", cfg.Buckets.Frontend)),
	}))
	project.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions:   jsii.Strings("cloudfront:CreateInvalidation"),
		Resources: jsii.Strings(fmt.Sprintf("arn:aws:cloudfront::%s:distribution/*", cfg.AccountID)),
	}))
}
