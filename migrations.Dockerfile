FROM migrate/migrate

RUN mkdir migrations

COPY migrations migrations

ENTRYPOINT ["migrate"]