# kexport

Google Kubernetes Engine usage export to BigQuery (based on gke-billing-export). The executable takes no arguments, it derives project ID from metadata server and polls all clusters in that project (default every min) and loads extracted data (pod, reserved and used cpu, reserved and used ram) into BigQuery table (default `kadvice.metrics`). The default dataset and table as well as polling interval can overitten using `DATASET`, `TABLE`, and `INTERVAL` env vars respectively.

This is not an officially supported Google product.

## Install

```shell
PROJECT=$(gcloud config get-value project)
kubectl run kexport --env="INTERVAL=30s" \
		--replicas=1 --generator=run-pod/v1 \
		--image="gcr.io/${PROJECT}-public/kexport:0.3.3"
```

# Example Output

Example of data exported by this tool into BigQuery.

```sql
SELECT * FROM `cloudylabs.kadvice.metrics`
WHERE namespace = 'demo'
```

Returns

```shell
| timestamp                      | project    | cluster | namespace | serviceaccount | pod                                    | reserved_cpu | reserved_ram | used_cpu | used_ram |
| ------------------------------ | ---------- | ------- | --------- | -------------- | -------------------------------------- | ------------ | ------------ | -------- | -------- |
| 2019-06-11 03:29:22.902388 UTC | cloudylabs | cr      | demo      | default        | kdemo-x6rfc-deployment-8fc5bfb9f-72nwx | 25           | 0            | 5        | 16003072 |
2019-06-11 03:50:31.59655 UTC	cloudylabs	cr	demo	default	kdemo-x6rfc-deployment-8fc5bfb9f-4xmss	25	0	4	15835136
2019-06-11 03:49:31.528159 UTC	cloudylabs	cr	demo	default	kdemo-x6rfc-deployment-8fc5bfb9f-4xmss	25	0	0	0
2019-06-11 04:01:49.499 UTC	cloudylabs	cr	demo	default	kdemo-x6rfc-deployment-8fc5bfb9f-tq55p	25	0	5	16642048
2019-06-11 04:00:49.499161 UTC	cloudylabs	cr	demo	default	kdemo-x6rfc-deployment-8fc5bfb9f-tq55p	25	0	5	16072704
```


```sql
SELECT project, cluster, namespace, AVG(used_ram) AS reserved_ram, AVG(used_cpu) AS reserved_cpu
FROM kadvice.metrics
GROUP BY project, cluster, namespace
ORDER BY 1, 2 desc
```

Waiting on bqjob_r9b4447e01a43528_00000165561be752_1 ... (0s) Current status: DONE
+------------------------------+------------------+-------------+--------------+--------------+
|           project            |     cluster      |  namespace  | reserved_ram | reserved_cpu |
+------------------------------+------------------+-------------+--------------+--------------+
| gke-billing-export-213323    | regional-cluster | default     |   6186598400 |        11800 |
| web-application-service-demo | cluster-1        | webapp-prod |   1258291200 |         2400 |
| web-application-service-demo | cluster-1        | webapp-test |    209715200 |          400 |
| web-application-service-demo | cluster-1        | vault       |    209715200 |          400 |
+------------------------------+------------------+-------------+--------------+--------------+
```

## Disclaimer
