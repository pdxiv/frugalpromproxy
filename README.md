# FrugalPromProxy

Simple attempt to see if the volume of metrics from Prometheus can be lessened by not sending metrics that don't change often or ever.

Usage example:
`./frugalpromproxy 9100 19100`

This will scrape port 9100 (node exporter) locally and expose a "slimmed down" version of the metrics on port 19100 which doesn't contain metrics that haven't changed value recently.
