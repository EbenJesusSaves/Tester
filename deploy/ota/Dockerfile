# Build the repository's existing image first; this layer only loads mounted secrets.
ARG OTA_BASE_IMAGE=xprem-ota-base:local
FROM ${OTA_BASE_IMAGE}
COPY --chmod=0555 deploy/ota/entrypoint.sh /app/ota-entrypoint.sh
ENTRYPOINT ["/app/ota-entrypoint.sh"]
CMD ["/app/main"]
