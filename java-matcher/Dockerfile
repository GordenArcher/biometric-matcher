# Multi-stage build, the Gradle wrapper and full JDK are only needed to
# produce the jar, the runtime image should just be a JRE running it.
FROM eclipse-temurin:21-jdk AS build
WORKDIR /app
COPY . .
RUN ./gradlew installDist --no-daemon

FROM eclipse-temurin:21-jre
WORKDIR /app
COPY --from=build /app/build/install/biometric-matcher /app
EXPOSE 50051
ENTRYPOINT ["/app/bin/biometric-matcher"]
