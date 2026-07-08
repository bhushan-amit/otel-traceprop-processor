ECR_REPO   := 869420678547.dkr.ecr.ap-south-1.amazonaws.com/otel-collector-custom
EB_APP     := otel-collector
EB_ENV     := Otel-collector-env-1
S3_BUCKET  := elasticbeanstalk-ap-south-1-869420678547
S3_PREFIX  := otel-collector-custom
REGION     := ap-south-1
LABEL      := deploy-$(shell date +%Y%m%d_%H%M%S)

.PHONY: build push deploy all

build:
	docker buildx build \
		--platform linux/arm64 \
		--tag $(ECR_REPO):latest \
		--push .

deploy:
	cd eb-deploy && \
	zip -j eb-bundle.zip Dockerrun.aws.json && \
	aws s3 cp eb-bundle.zip s3://$(S3_BUCKET)/$(S3_PREFIX)/$(LABEL).zip --region $(REGION) && \
	aws elasticbeanstalk create-application-version \
		--region $(REGION) \
		--application-name $(EB_APP) \
		--version-label "$(LABEL)" \
		--source-bundle S3Bucket=$(S3_BUCKET),S3Key=$(S3_PREFIX)/$(LABEL).zip && \
	aws elasticbeanstalk update-environment \
		--region $(REGION) \
		--environment-name $(EB_ENV) \
		--version-label "$(LABEL)"

ecr-login:
	aws ecr get-login-password --region $(REGION) | \
		docker login --username AWS --password-stdin 869420678547.dkr.ecr.ap-south-1.amazonaws.com

all: ecr-login build deploy

status:
	@aws elasticbeanstalk describe-environment-health \
		--region $(REGION) --environment-name $(EB_ENV) \
		--attribute-names All \
		--query '{Health:HealthStatus,Status:Status,Ok:InstancesHealth.Ok,Degraded:InstancesHealth.Degraded}' \
		--output json
	@aws elbv2 describe-target-health \
		--target-group-arn arn:aws:elasticloadbalancing:$(REGION):869420678547:targetgroup/otel-grpc-4317-prod/6da1acaa9a11fe40 \
		--region $(REGION) \
		--query 'TargetHealthDescriptions[*].{Id:Target.Id,State:TargetHealth.State}' \
		--output table