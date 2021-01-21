# Gitlab Runner Resources Webhook

This project should not be required and is a workaround to [this issue](https://gitlab.com/gitlab-org/gitlab-runner/-/issues/3464). You might want to also monitor [this MR](https://gitlab.com/gitlab-org/gitlab-runner/-/merge_requests/1526), once it's merged this repo is pointless.

It was also a good exercise to learn more about writing Admission Webhooks.

## Setup

The webhook works for pods created by a [gitlab runner](https://docs.gitlab.com/runner/) using the
[kubernetes executor](https://docs.gitlab.com/runner/executors/kubernetes.html). 
```bash
kubectl -n gitlab apply -f deploy/
kubectl -n gitlab certificate approve gitlab-resources-webhook.gitlab
```

Some relevant points:
* This setup was tested with version 0.25.0 of the gitlab runner - there is a requirement that the main job container is named `build`, changes to this in the future would require updates on the webhook code
* The commands above create resources in the gitlab namespace - if you change
this, you'll need to also update the namespace reference to the Service in
the MutatingWebhookConfiguration and the ServiceAccount in the Job
definition 
* The objectSelector in the MutatingWebhookConfiguration sets the expected
label for pods that should trigger the webhook. The value is preset to `gpu=true`
but you can change this to whatever pod_labels you have in your runner config
- which should look something like this:
```toml
[[runners]]
  [runners.kubernetes]
    image = "ubuntu:16.04"
    namespace_overwrite_allowed = ""
    [runners.kubernetes.pod_labels]
      gpu = "true"
```
