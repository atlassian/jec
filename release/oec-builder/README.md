Example build command:

docker build \
    -t jec-builder \
    --build-arg GO_VERSION=1.12.1 \
    --no-cache .

Example run command:

docker run \
 --entrypoint /input/build \
 -e JEC_VERSION=1.0.3 \
 -e JEC_REPO=/jec_repo \
 -e OUTPUT=/jec_repo/release/jec-builder \
 -v /Users/faziletozer/go/src/github.com/atlassian/jec:/jec_repo \
 -v $(pwd):/input \
 jec-builder

Run docker run command in jec-linux, jec-win32 and jec-win64 folders, the executables will be generated under jec-packages folder.