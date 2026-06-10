// Package main demonstrates deploying google-group-sync as an AWS Lambda function
// using Pulumi Go SDK. The Lambda is deployed with:
// - ARM64 (Graviton) for cost efficiency
// - Lambda Web Adapter (LWA) layer for HTTP translation
// - Function URL with AWS_IAM authentication
// - Google SA key passed as environment variable (marked as secret)
//
// Prerequisites:
// - AWS credentials configured
// - Google Workspace service account key JSON
//
// Usage:
//
//	cd deploy/example
//	pulumi up
package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/lambda"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	// Lambda Web Adapter layer ARN (arm64, eu-central-1).
	// See: https://github.com/awslabs/aws-lambda-web-adapter
	// Find your region's ARN at: https://github.com/awslabs/aws-lambda-web-adapter#lambda-layer
	lwaLayerARN = "arn:aws:lambda:eu-central-1:753240598075:layer:LambdaAdapterLayerArm64:25"

	// Version of google-group-sync to deploy.
	version = "0.1.1"

	// GitHub Release ZIP URL.
	zipURL = "https://github.com/truvity/google-group-sync/releases/download/v" + version + "/google-group-sync_" + version + "_linux_arm64.zip"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		// Configuration values (set via `pulumi config set`).
		googleAdminEmail := cfg.Require("googleAdminEmail")
		googleSAKeyJSON := cfg.RequireSecret("googleSAKeyJSON")

		// IAM role for the Lambda function.
		lambdaRole, err := iam.NewRole(ctx, "google-group-sync-role", &iam.RoleArgs{
			Name: pulumi.String("google-group-sync"),
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Action": "sts:AssumeRole",
					"Effect": "Allow",
					"Principal": {"Service": "lambda.amazonaws.com"}
				}]
			}`),
		})
		if err != nil {
			return fmt.Errorf("create IAM role: %w", err)
		}

		// Attach basic execution policy (CloudWatch Logs).
		_, err = iam.NewRolePolicyAttachment(ctx, "google-group-sync-basic", &iam.RolePolicyAttachmentArgs{
			Role:      lambdaRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		})
		if err != nil {
			return fmt.Errorf("attach basic execution policy: %w", err)
		}

		// CloudWatch Log Group.
		logGroup, err := cloudwatch.NewLogGroup(ctx, "google-group-sync-logs", &cloudwatch.LogGroupArgs{
			Name:            pulumi.String("/aws/lambda/google-group-sync"),
			RetentionInDays: pulumi.Int(7),
		})
		if err != nil {
			return fmt.Errorf("create log group: %w", err)
		}

		// Lambda function — uses public GitHub Release ZIP directly.
		fn, err := lambda.NewFunction(ctx, "google-group-sync", &lambda.FunctionArgs{
			Name:    pulumi.String("google-group-sync"),
			Role:    lambdaRole.Arn,
			Runtime: pulumi.String("provided.al2023"),
			Architectures: pulumi.StringArray{
				pulumi.String("arm64"),
			},
			Handler:    pulumi.String("bootstrap"),
			Timeout:    pulumi.Int(10),
			MemorySize: pulumi.Int(128),
			Code:       pulumi.NewRemoteArchive(zipURL),
			Layers: pulumi.StringArray{
				pulumi.String(lwaLayerARN),
			},
			LoggingConfig: &lambda.FunctionLoggingConfigArgs{
				LogGroup:  logGroup.Name,
				LogFormat: pulumi.String("JSON"),
			},
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					"PORT":                         pulumi.String("8080"),
					"HEALTH_PORT":                  pulumi.String("7070"),
					"AWS_LWA_READINESS_CHECK_PATH": pulumi.String("/health"),
					"AWS_LWA_READINESS_CHECK_PORT": pulumi.String("7070"),
					"AWS_LAMBDA_EXEC_WRAPPER":      pulumi.String("/opt/bootstrap"),
					"AWS_LWA_ASYNC_INIT":           pulumi.String("true"),
					"GOOGLE_ADMIN_EMAIL":           pulumi.String(googleAdminEmail),
					"GOOGLE_SA_KEY_JSON":           googleSAKeyJSON,
					"LOG_FORMAT":                   pulumi.String("json"),
					"CACHE_TTL":                    pulumi.String("5m"),
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{logGroup}))
		if err != nil {
			return fmt.Errorf("create Lambda function: %w", err)
		}

		// Function URL with AWS_IAM auth (callers must have lambda:InvokeFunctionUrl permission).
		fnURL, err := lambda.NewFunctionUrl(ctx, "google-group-sync-url", &lambda.FunctionUrlArgs{
			FunctionName:      fn.Name,
			AuthorizationType: pulumi.String("AWS_IAM"),
		})
		if err != nil {
			return fmt.Errorf("create Function URL: %w", err)
		}

		// Exports.
		ctx.Export("functionName", fn.Name)
		ctx.Export("functionArn", fn.Arn)
		ctx.Export("functionUrl", fnURL.FunctionUrl)

		return nil
	})
}
