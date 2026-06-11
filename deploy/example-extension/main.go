// Package main demonstrates deploying google-group-sync as a Lambda Extension
// (sidecar) using Pulumi Go SDK. The extension runs alongside a host Lambda
// function and serves groups resolution via localhost:9090.
//
// Architecture:
//
//	[Host Lambda (your-webhook)]
//	  ├── LWA layer (event→HTTP)
//	  ├── google-group-sync extension layer (HTTP on :9090)
//	  │     └── reads GGS_SA_KEY_SECRET_NAME from Secrets Manager at startup
//	  └── host binary (calls http://localhost:9090/groups)
//
// All extension env vars use the GGS_ prefix to avoid collisions with the host.
//
// Prerequisites:
// - AWS credentials configured
// - Google Workspace service account key JSON stored in Secrets Manager
// - Host Lambda ZIP in S3
//
// Usage:
//
//	cd deploy/example-extension
//	pulumi up
package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	// Lambda Web Adapter layer ARN (arm64, eu-central-1).
	lwaLayerARN = "arn:aws:lambda:eu-central-1:753240598075:layer:LambdaAdapterLayerArm64:25"

	// Version of google-group-sync extension to deploy.
	version = "0.4.0"

	// GitHub Release extension ZIP URL.
	extensionZipURL = "https://github.com/truvity/google-group-sync/releases/download/v" + version + "/google-group-sync_extension_" + version + "_arm64.zip"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		// Configuration values.
		googleAdminEmail := cfg.Require("googleAdminEmail")
		googleSAKeyJSON := cfg.RequireSecret("googleSAKeyJSON")
		hostLambdaS3Bucket := cfg.Require("hostLambdaS3Bucket")
		hostLambdaS3Key := cfg.Require("hostLambdaS3Key")

		// ── SA Key Secret ──────────────────────────────────────────────────

		saKeySecret, err := secretsmanager.NewSecret(ctx, "google-sa-key", &secretsmanager.SecretArgs{
			Name:                 pulumi.String("google-group-sync/sa-key"),
			RecoveryWindowInDays: pulumi.Int(0),
		})
		if err != nil {
			return fmt.Errorf("create SA key secret: %w", err)
		}

		_, err = secretsmanager.NewSecretVersion(ctx, "google-sa-key-version", &secretsmanager.SecretVersionArgs{
			SecretId:     saKeySecret.ID(),
			SecretString: googleSAKeyJSON,
		})
		if err != nil {
			return fmt.Errorf("create SA key secret version: %w", err)
		}

		// ── IAM Role ───────────────────────────────────────────────────────

		lambdaRole, err := iam.NewRole(ctx, "host-lambda-role", &iam.RoleArgs{
			Name: pulumi.String("my-webhook-with-extension"),
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

		_, err = iam.NewRolePolicyAttachment(ctx, "host-lambda-basic", &iam.RolePolicyAttachmentArgs{
			Role:      lambdaRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		})
		if err != nil {
			return fmt.Errorf("attach basic execution policy: %w", err)
		}

		// Secrets Manager read policy (extension reads SA key at init).
		smPolicy, err := iam.NewRolePolicy(ctx, "host-lambda-sm-read", &iam.RolePolicyArgs{
			Role: lambdaRole.Name,
			Policy: saKeySecret.Arn.ApplyT(func(arn string) string {
				return fmt.Sprintf(`{
					"Version": "2012-10-17",
					"Statement": [{
						"Effect": "Allow",
						"Action": ["secretsmanager:GetSecretValue"],
						"Resource": [%q]
					}]
				}`, arn)
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return fmt.Errorf("create SM read policy: %w", err)
		}

		// ── Extension Layer ────────────────────────────────────────────────

		// Publish the extension ZIP as a Lambda Layer.
		// The ZIP contains extensions/google-group-sync which Lambda auto-discovers.
		extensionLayer, err := lambda.NewLayerVersion(ctx, "google-group-sync-extension", &lambda.LayerVersionArgs{
			LayerName:   pulumi.String("google-group-sync-extension"),
			Description: pulumi.Sprintf("google-group-sync v%s extension", version),
			Content:     pulumi.NewRemoteArchive(extensionZipURL),
			CompatibleRuntimes: pulumi.StringArray{
				pulumi.String("provided.al2023"),
			},
			CompatibleArchitectures: pulumi.StringArray{
				pulumi.String("arm64"),
			},
		})
		if err != nil {
			return fmt.Errorf("create extension layer: %w", err)
		}

		// ── Host Lambda ────────────────────────────────────────────────────

		logGroup, err := cloudwatch.NewLogGroup(ctx, "host-lambda-logs", &cloudwatch.LogGroupArgs{
			Name:            pulumi.String("/aws/lambda/my-webhook"),
			RetentionInDays: pulumi.Int(7),
		})
		if err != nil {
			return fmt.Errorf("create log group: %w", err)
		}

		// The host Lambda includes both the LWA layer and the extension layer.
		// Extension env vars use the GGS_ prefix.
		fn, err := lambda.NewFunction(ctx, "host-lambda", &lambda.FunctionArgs{
			Name:    pulumi.String("my-webhook"),
			Role:    lambdaRole.Arn,
			Runtime: pulumi.String("provided.al2023"),
			Architectures: pulumi.StringArray{
				pulumi.String("arm64"),
			},
			Handler:    pulumi.String("bootstrap"),
			Timeout:    pulumi.Int(30), // Must be > 10s to allow extension cold start (SM fetch)
			MemorySize: pulumi.Int(128),
			S3Bucket:   pulumi.String(hostLambdaS3Bucket),
			S3Key:      pulumi.String(hostLambdaS3Key),
			Layers: pulumi.StringArray{
				pulumi.String(lwaLayerARN),
				extensionLayer.Arn,
			},
			LoggingConfig: &lambda.FunctionLoggingConfigArgs{
				LogGroup:  logGroup.Name,
				LogFormat: pulumi.String("JSON"),
			},
			Environment: &lambda.FunctionEnvironmentArgs{
				Variables: pulumi.StringMap{
					// Host Lambda env vars (no prefix).
					"PORT":                         pulumi.String("8080"),
					"HEALTH_PORT":                  pulumi.String("7070"),
					"AWS_LWA_READINESS_CHECK_PATH": pulumi.String("/health"),
					"AWS_LWA_READINESS_CHECK_PORT": pulumi.String("7070"),
					"AWS_LAMBDA_EXEC_WRAPPER":      pulumi.String("/opt/bootstrap"),
					"AWS_LWA_ASYNC_INIT":           pulumi.String("true"),
					"GROUPS_RESOLVER_URL":          pulumi.String("http://localhost:9090/groups"),

					// Extension env vars (GGS_ prefix — avoids collision with host).
					"GGS_GOOGLE_ADMIN_EMAIL": pulumi.String(googleAdminEmail),
					"GGS_SA_KEY_SECRET_NAME": pulumi.String("google-group-sync/sa-key"),
					"GGS_PORT":               pulumi.String("9090"),
					"GGS_HEALTH_PORT":        pulumi.String("0"),
					"GGS_LOG_FORMAT":         pulumi.String("json"),
					"GGS_CACHE_TTL":          pulumi.String("5m"),
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{logGroup, smPolicy}))
		if err != nil {
			return fmt.Errorf("create host Lambda: %w", err)
		}

		// Function URL with AWS_IAM auth.
		fnURL, err := lambda.NewFunctionUrl(ctx, "host-lambda-url", &lambda.FunctionUrlArgs{
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
		ctx.Export("extensionLayerArn", extensionLayer.Arn)
		ctx.Export("saKeySecretArn", saKeySecret.Arn)

		return nil
	})
}
