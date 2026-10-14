FROM python:3.13-alpine

# The client prints each message as it arrives; buffered output would hold them until it exits.
ENV PYTHONUNBUFFERED=1

RUN pip install --no-cache-dir async-upnp-client==0.49.0

ENTRYPOINT ["upnp-client"]
