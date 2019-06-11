# kexport

GKE pod metric export to BigQuery (based on [professional-services](https://github.com/mchmarny/professional-services)). Single container which derives project ID from metadata server and polls all clusters in that project every min and then loads extracted data (pod, reserved and used cpu, reserved and used ram) into BigQuery table (default `kadvice.metrics`). The default dataset as well as the table and polling interval can overwritten using `DATASET`, `TABLE`, and `INTERVAL` env vars respectively.

## Deployment

```shell
PROJECT=$(gcloud config get-value project)
kubectl run kexport --env="INTERVAL=30s" \
	--replicas=1 --generator=run-pod/v1 \
	--image="gcr.io/${PROJECT}-public/kexport:0.3.3"
```

# Example Output

Example of data exported by this tool into BigQuery.

```sql
SELECT * FROM `kadvice.metrics`
```

Outputs

```shell
| metric_time                    | project    | cluster | namespace       | serviceaccount | pod                                 | reserved_cpu | reserved_ram | used_cpu | used_ram |
| ------------------------------ | ---------- | ------- | --------------- | -------------- | ----------------------------------- | ------------ | ------------ | -------- | -------- |
| 2019-06-11 14:30:52.738756 UTC | cloudylabs | cr      | knative-serving | controller     | webhook-759c4676bd-c2nqz            | 20           | 20971520     | 3        | 7917568  |
| 2019-06-11 14:02:51.535108 UTC | cloudylabs | cr      | knative-serving | controller     | cloudrun-controller-7956cd9b6-wptnx | 0            | 0            | 4        | 9052160  |
| 2019-06-11 14:30:52.738749 UTC | cloudylabs | cr      | knative-serving | controller     | controller-5ff95479f5-kdshq         | 100          | 104857600    | 27       | 21028864 |
| 2019-06-11 14:45:22.749121 UTC | cloudylabs | cr      | knative-serving | controller     | networking-istio-b7cccb5fb-wsm2s    | 100          | 104857600    | 5        | 12193792 |
| 2019-06-11 14:00:51.481584 UTC | cloudylabs | cr      | knative-serving | controller     | webhook-759c4676bd-c2nqz            | 20           | 20971520     | 2        | 7917568  |
| 2019-06-11 14:12:21.481379 UTC | cloudylabs | cr      | knative-serving | controller     | networking-istio-b7cccb5fb-wsm2s    | 100          | 104857600    | 6        | 12193792 |
```

Similarly you can generate application-level aggregate reports (e.g. average RAM and CPU used by specific app)

```sql
SELECT
  FORMAT_TIMESTAMP("%F-%H-%M", metric_time) as metric_hour,
  AVG(used_ram) AS avg_used_ram,
  AVG(used_cpu) AS avg_used_cpu
FROM kadvice.metrics
WHERE
  project = 'cloudylabs' AND
  cluster = 'cr' AND
  namespace = 'demo' AND
  pod like 'kdemo%'
GROUP BY metric_hour
ORDER BY 1 desc
```

Outputs

```shell
| metric_hour 	    | avg_used_ram 	| avg_used_cpu 	|
|------------------	|-------------:	|-------------:	|
| 2019-06-11-15-16 	| 19533824 	    | 5 	        |
| 2019-06-11-15-15 	| 17096704 	    | 6 	        |
| 2019-06-11-15-14 	| 8044544 	    | 2.5 	        |
| 2019-06-11-15-02 	| 17653760 	    | 4.5 	        |
| 2019-06-11-15-01 	| 16392192 	    | 6 	        |
| 2019-06-11-15-00 	| 7905280 	    | 2.5 	        |
```


## Disclaimer

This is my personal project and it does not represent my employer. I take no responsibility for issues caused by this code. I do my best to ensure that everything works, but if something goes wrong, my apologies is all you will get.