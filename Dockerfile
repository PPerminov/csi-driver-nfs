FROM centos:7.7.1908
ENTRYPOINT ["/nfsplugin"]
RUN yum -y install nfs-utils epel-release && yum -y install jq && yum clean all
COPY bin/nfsplugin /nfsplugin