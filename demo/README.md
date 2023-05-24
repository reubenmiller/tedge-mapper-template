# Demo

## Getting started

1. Build then copy the artifacts from the experimental thin-edge.io branch - [exp-topic-prefix](https://github.com/reubenmiller/thin-edge.io/tree/exp-topic-prefix)

    ```sh
    # from the thin-edge.io repo
    just release
    ```

    Then copy the built debian packages to this project

    ```sh
    cp target/aarch64-unknown-linux-musl/debian/*.deb tedge-mapper-template/demo/dist
    ```

2. Build the latest tedge-mapper-template (from this repo) and copy the specific artifact to the `demo/dist` folder

    ```sh
    just release-local
    cp dist/tedge-mapper-template_*arm64*deb demo/dist/
    ```

    Note: You will have to change the `arm64` to suite your architecture of your current setup

2. Start the docker compose project

    ```sh
    cd demo
    just up
    ```

3. View the logs for the tedge-mapper-template running on the main device

    ```sh
    just logs-main-mapper-template
    ```

4. Open a new console and log the tedge-agent running on the child device

    ```sh
    cd demo
    just logs-child01-agent
    ```

5. Create a software update operation for the child01 device in Cumulocity

    You can use `go-c8y-cli` to create the operation if you don't want to use the UI

    ```sh
    c8y operations create --device "{child__mo_id}" --description "install software" --template "{c8y_SoftwareUpdate:[{name:'dummy1',version:'1.0.0::dummy',url:'',softwareType:'dummy',action:'install'}]}"
    ```

    Example

    ```sh
    c8y operations create --device "862000795" --description "install software" --template "{c8y_SoftwareUpdate:[{name:'dummy1',version:'1.0.0::dummy',url:'',softwareType:'dummy',action:'install'}]}"
    ```

    If everything works properly you should see the software update operation be handled successfully by the `tedge-agent` and the `tedge-mapper-template` is doing all of the cloud message routing.

6. You can stop the demo by using

    ```sh
    just down
    ```
