# Multi-stage build. Uses Gradle's own image rather than a wrapper script,
# there's no gradlew committed here (generating one needs a real gradle
# binary and network access to Gradle's distribution servers, neither
# available in the environment this scaffold was built in), this avoids
# that dependency entirely for the Docker path.
FROM gradle:8.9-jdk21 AS build
WORKDIR /app
COPY . .
RUN gradle installDist --no-daemon

FROM eclipse-temurin:21-jre
WORKDIR /app
COPY --from=build /app/build/install/biometric-matcher /app
EXPOSE 50051
ENTRYPOINT ["/app/bin/biometric-matcher"]