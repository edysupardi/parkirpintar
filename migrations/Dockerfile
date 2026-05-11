FROM migrate/migrate:v4.17.0

COPY . /migrations

ENTRYPOINT ["migrate", "-path", "/migrations", "-database"]
