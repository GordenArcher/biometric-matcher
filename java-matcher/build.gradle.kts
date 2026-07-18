plugins {
    id("java")
    id("application")
    id("com.google.protobuf") version "0.9.4"
}

group = "com.gordenarcher.biometric"
version = "0.1.0"

repositories {
    mavenCentral()
}

// Pinned together deliberately, grpc-java and protobuf-java versions are
// tightly coupled, bumping one without the other is a common source of
// runtime ClassCastExceptions that don't show up until you actually call
// an RPC.
val grpcVersion = "1.65.1"
val protobufVersion = "3.25.3"

dependencies {
    implementation("io.grpc:grpc-netty-shaded:$grpcVersion")
    implementation("io.grpc:grpc-protobuf:$grpcVersion")
    implementation("io.grpc:grpc-stub:$grpcVersion")
    implementation("com.google.protobuf:protobuf-java:$protobufVersion")

    // javax.annotation.Generated is required by generated grpc stubs on
    // JDK 11+, it was removed from the JDK itself in 9. Easy to forget
    // and get a confusing compile error without it.
    compileOnly("org.apache.tomcat:annotations-api:6.0.53")

    // The actual matcher. Pin the version explicitly rather than a range,
    // a silent matcher algorithm change between versions is not something
    // you want happening on a routine `./gradlew build`.
    implementation("com.machinezoo.sourceafis:sourceafis:3.16.1")

    testImplementation(platform("org.junit:junit-bom:5.10.2"))
    testImplementation("org.junit.jupiter:junit-jupiter")
}

application {
    mainClass.set("com.gordenarcher.biometric.MatcherServer")
}

protobuf {
    protobuf.protoc {
        artifact = "com.google.protobuf:protoc:$protobufVersion"
    }
    protobuf.plugins {
        create("grpc") {
            artifact = "io.grpc:protoc-gen-grpc-java:$grpcVersion"
        }
    }
    protobuf.generateProtoTasks {
        all().forEach {
            it.plugins {
                create("grpc")
            }
        }
    }
}

tasks.test {
    useJUnitPlatform()
}
