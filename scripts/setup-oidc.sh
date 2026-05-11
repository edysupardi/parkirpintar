#!/usr/bin/env bash
# Setup OIDC provider and IAM role for GitHub Actions deployment.
# Run once before first deploy.
# Usage: ./scripts/setup-oidc.sh <AWS_ACCOUNT_ID>

set -euo pipefail

ACCOUNT_ID="${1:?Usage: $0 <AWS_ACCOUNT_ID>}"
ROLE_NAME="edysup-parkirpintar-deploy"
REPO="edysupardi/parkirpintar"
REGION="ap-southeast-1"

echo "Creating OIDC provider for GitHub Actions..."
aws iam create-open-id-connect-provider \
  --url "https://token.actions.githubusercontent.com" \
  --client-id-list "sts.amazonaws.com" \
  --thumbprint-list "6938fd4d98bab03faadb97b34396831e3780aea1" \
  --region "$REGION" 2>/dev/null || echo "OIDC provider already exists"

echo "Creating IAM role: $ROLE_NAME"
TRUST_POLICY=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::${ACCOUNT_ID}:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:${REPO}:*"
        }
      }
    }
  ]
}
EOF
)

aws iam create-role \
  --role-name "$ROLE_NAME" \
  --assume-role-policy-document "$TRUST_POLICY" \
  --tags Key=Project,Value=edysup-parkirpintar Key=ManagedBy,Value=script \
  2>/dev/null || echo "Role already exists"

echo "Attaching policies..."
for policy in \
  "arn:aws:iam::aws:policy/AmazonECS_FullAccess" \
  "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryFullAccess" \
  "arn:aws:iam::aws:policy/CloudWatchLogsFullAccess"; do
  aws iam attach-role-policy --role-name "$ROLE_NAME" --policy-arn "$policy"
done

ROLE_ARN="arn:aws:iam::${ACCOUNT_ID}:role/${ROLE_NAME}"
echo ""
echo "Done! Set this in GitHub Secrets:"
echo "  AWS_ROLE_ARN = $ROLE_ARN"
echo ""
echo "Then set in GitHub Environment (dev/production):"
echo "  AWS_ACCOUNT_ID = $ACCOUNT_ID"
