locals {
  command_queue_arns = [
    for name in var.command_queue_names :
    "arn:aws:sqs:${var.region}:${data.aws_caller_identity.current.account_id}:${name}"
  ]
}

data "aws_iam_policy_document" "pod_assume_role" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [local.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider}:sub"
      values   = ["system:serviceaccount:${var.kubernetes_namespace}:${var.kubernetes_service_account}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "pod" {
  statement {
    sid = "ConsumeSagaReplies"
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:ChangeMessageVisibility",
      "sqs:GetQueueAttributes",
      "sqs:GetQueueUrl",
    ]
    resources = [aws_sqs_queue.saga_replies.arn]
  }

  statement {
    sid       = "PublishSagaCommands"
    actions   = ["sqs:SendMessage", "sqs:GetQueueUrl"]
    resources = local.command_queue_arns
  }
}

resource "aws_iam_role" "pod" {
  name               = "${var.service_name}-pod"
  description        = "Assumed by the pods of this microservice through the cluster OIDC provider (IRSA)"
  assume_role_policy = data.aws_iam_policy_document.pod_assume_role.json
}

resource "aws_iam_role_policy" "pod" {
  name   = "${var.service_name}-pod"
  role   = aws_iam_role.pod.id
  policy = data.aws_iam_policy_document.pod.json
}
