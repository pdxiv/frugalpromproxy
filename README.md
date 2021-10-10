# FrugalPromProxy

Simple attempt to see if the volume of metrics from Prometheus can be lessened by not sending metrics that don't change often or ever.

Usage example:
`./frugalpromproxy 9100 19100`

This will scrape port 9100 locally and expose a "slimmed down" version of the metrics on port 19100.
