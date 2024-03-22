FROM formulatelbase

COPY /out/rpc /usr/bin
ENTRYPOINT ["rpc"]