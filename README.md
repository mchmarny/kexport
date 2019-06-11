# kexport

Google Kubernetes Engine usage export to BigQuery (based on [professional-services](https://github.com/mchmarny/professional-services)). The executable takes no arguments, it derives project ID from metadata server and polls all clusters in that project (default every min) and loads extracted data (pod, reserved and used cpu, reserved and used ram) into BigQuery table (default `kadvice.metrics`). The default dataset and table as well as polling interval can overitten using `DATASET`, `TABLE`, and `INTERVAL` env vars respectively.

This is not an officially supported Google product.

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
| metric_time 	| project 	| cluster 	| namespace 	| serviceaccount 	| pod 	| reserved_cpu 	| reserved_ram 	| used_cpu 	| used_ram 	|
|--------------------------------	    |------------	|---------	|-----------	|----------------	|-------------------------------------------- |-------------:	|-------------:	|---------:	|---------:	|
| 2019-06-11 15:02:22.800456 UTC 	    | cloudylabs 	| cr 	| demo 	| default 	| kdemo-x6rfc-deployment-8fc5bfb9f-zvp77 	| 25 	| 0 	| 5 	| 17649664 	|
| 2019-06-11 15:01:52.75644 UTC 	    | cloudylabs 	| cr 	| demo 	| default 	| maxprime-nwgnk-deployment-7567cd9ccc-9z9d9 	    | 25 	| 0 	| 4 	| 18292736 	|
| 2019-06-11 15:01:52.756437 UTC 	    | cloudylabs 	| cr 	| demo 	| default 	| kdemo-x6rfc-deployment-8fc5bfb9f-zvp77 	| 25 	| 0 	| 7 	| 16961536 	|
| 2019-06-11 15:01:22.762182 UTC 	    | cloudylabs 	| cr 	| demo 	| default 	| maxprime-nwgnk-deployment-7567cd9ccc-9z9d9 	    | 25 	| 0 	| 5 	| 18030592 	|
| 2019-06-11 15:01:22.76218 UTC 	    | cloudylabs 	| cr 	| demo 	| default 	| kuser-klzg6-deployment-6f9dcf64b7-sfbhw 	    | 25 	| 0 	| 6 	| 22835200 	|
| 2019-06-11 15:01:22.762178 UTC 	    | cloudylabs 	| cr 	| demo 	| default 	| klogo-kfmq5-deployment-7656dc7769-njgv5 	    | 25 	| 0 	| 6 	| 18571264 	|
| 2019-06-11 15:01:22.762175 UTC 	    | cloudylabs 	| cr 	| demo 	| default 	| kdemo-x6rfc-deployment-8fc5bfb9f-zvp77 	| 25 	| 0 	| 5 	| 15822848 	|
```

Similarly you can generate application-level aggrade reports (e.g. average RAM and CPU used by specific app)

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