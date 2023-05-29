FROM rockylinux:8 AS builder

# Set environment variables
ENV RANGER_VERSION=2.4.0
ENV MAVEN_VERSION=3.6.3

RUN dnf install -y java-1.8.0-openjdk-devel patch git npm gcc python3 wget zip unzip bash \
  && echo 2 | alternatives --config python

RUN wget http://apache.rediris.es/ranger/${RANGER_VERSION}/apache-ranger-${RANGER_VERSION}.tar.gz -O ranger.tar.gz
COPY 0.patch /0.patch
RUN mkdir /ranger \
  && tar xvzf /ranger.tar.gz -C /ranger
RUN ls /ranger

RUN wget https://dlcdn.apache.org/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz -P /tmp \
  && tar xf /tmp/apache-maven-${MAVEN_VERSION}-bin.tar.gz -C /opt \
  && ln -s /opt/apache-maven-${MAVEN_VERSION} /opt/maven

ENV MAVEN_HOME /usr/share/maven 
ENV M2_HOME /opt/maven 
ENV PATH ${M2_HOME}/bin:${PATH}
ENV JAVA_HOME /usr/lib/jvm/jre-openjdk
ENV PYTHON3 /usr/bin/python3
ENV PATH ${PATH}:${PYTHON3}

# Install SDKMAN
RUN curl -s "https://get.sdkman.io" | bash

# Set up environment variables for SDKMAN
ENV SDKMAN_DIR="/root/.sdkman"

# Make sure that bash is used and source sdkman-init.sh for SDKMAN to work
RUN source "$HOME/.sdkman/bin/sdkman-init.sh" && sdk install java 8.0.302-open && sdk use java 8.0.302-open

RUN cd /ranger/apache-ranger-${RANGER_VERSION} \
  && patch ./security-admin/src/main/resources/conf.dist/ranger-admin-default-site.xml /0.patch
RUN cd /ranger/apache-ranger-${RANGER_VERSION} \
  && mvn -Pall clean \
  && mvn -Pall -DskipTests clean compile package install

FROM openjdk:8-jdk-alpine

ARG RANGER_VERSION=2.4.0
ARG POSTGRES_JDBC_VERSION=42.2.18
ARG ZK_VERSION=3.5.7

RUN mkdir -p /usr/share/java \
  && mkdir -p /ranger /ranger/dist \
  && mkdir -p /ranger/backend_init/{zk,solr_collections}

ADD https://archive.apache.org/dist/zookeeper/zookeeper-${ZK_VERSION}/apache-zookeeper-${ZK_VERSION}-bin.tar.gz /apache-zookeeper-${ZK_VERSION}-bin.tar.gz
ADD https://jdbc.postgresql.org/download/postgresql-${POSTGRES_JDBC_VERSION}.jar /postgresql-connector-java.jar

COPY --from=builder /ranger/apache-ranger-${RANGER_VERSION}/target/*.tar.gz /ranger/dist/ 
COPY etc/pam.d/* /etc/pam.d/
COPY opt /opt/

RUN mv /postgresql-connector-java.jar /usr/share/java/postgresql-connector-java.jar
RUN tar xvzf apache-zookeeper-${ZK_VERSION}-bin.tar.gz
RUN apk add python py-jinja2 bash bc curl procps
RUN addgroup ranger \
  && adduser --ingroup ranger ranger -S \
  && for i in `ls /ranger/dist/*`;do mv $i ${i/${RANGER_VERSION}-/};done \
  && tar xvzf /ranger/dist/ranger-admin.tar.gz -C /ranger \
  && tar xvzf /ranger/dist/ranger-usersync.tar.gz -C /ranger \
  && ln -s /ranger/ranger-${RANGER_VERSION}-admin /ranger/ranger-admin \
  && ln -s /ranger/ranger-${RANGER_VERSION}-usersync /ranger/ranger-usersync \
  && ln -s /apache-zookeeper-3.5.7-bin /zookeeper

### specify default command
EXPOSE 6080/tcp
CMD /opt/keedio/bin/start.sh