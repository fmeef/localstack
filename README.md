# Localstack

Localstack aims to be a port of dan-v's [rattlesnakeos-stack](https://github.com/dan-v/rattlesnakeos-stack) build system from AWS to podman containers running on a local machine. Current, it builds an ota image capable of being sideloaded onto any device supported by rattlesnakeos-stack. 


## What works

- Building OTA image from rattlesnakeos and AOSP sources
- Incremental builds
- Key generation
- Force rebuild
- Custom patches

## What does not work

- OTA updates
- Attestation server
- Automatic builds

## Building
Localstack uses go modules, it can be build with `go build` with golang 1.14 or higher



## Compiling AOSP

Since localstack uses more or less the same build script as rattlesnakeos-stack, the same setup instructions apply for the first-time installtion of factory images. Read about it [here](https://github.com/dan-v/rattlesnakeos-stack/blob/11.0/FLASHING.md)

It should be noted that localstack current requires a *very* new version of podman supporting at least api version 2.0.0. A lot of systems do not support this in their current repos, requiring a manual installation of podman. 

### Generate configuration
``` sh
./localstack config
INFO[0000] Using config file: /home/user/.localstack.toml 
Device is the device codename (e.g. sailfish). Supported devices: sailfish (Pixel), marlin (Pixel XL), walleye (Pixel 2), taimen (Pixel 2 XL), blueline (Pixel 3), crosshatch (Pixel 3 XL), sargo (Pixel 3a), bonito (Pixel 3a XL)
Device : bonito
Path to store stateful files for local stack
State path : /home/
number of cpus to use for build (too many can result in OOM condition
Number of processors : 4
INFO[0007] rattlesnakeos-stack config file has been written to /home/user/.localstack.toml 
```

### Deploy podman build environent

``` sh
./localstack deploy
INFO[0000] Using config file: /home/user/.localstack.toml 
INFO[0000] Current settings:                            
chromium-version: ""
device: bonito
ignore-version-checks: false
nproc: "4"
statepath: /home/

? Do you want to continue ? [y/N] █
```




### Build aosp

``` sh
./localstack build
INFO[0000] Using config file: /home/user/.localstack.toml 
...
...
```


After a successful build, the OTA image should be written to `$STATE_PATH/.localstack/mounts/release`
