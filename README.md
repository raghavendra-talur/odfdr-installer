# odfdr-installer

## Overview

The `odfdr-installer` is a tool designed to facilitate the installation of the OpenShift Data Foundation (ODF) using custom sources. This tool streamlines the process of configuring necessary components and dependencies in an OpenShift environment.

## Prerequisites

- Go version 1.24.1 or higher.
- Ensure that the following commands are installed and available in your PATH:
  - `jq`
  - `oc` (OpenShift CLI)

## Installation

Clone this repository and build the project:

```bash
git clone https://github.com/raghavendra-talur/odfdr-installer.git
cd odfdr-installer
go build -o odfdr-installer
```

## Usage

To run the installer, execute the following command:

```bash
./odfdr-installer -url <URL> -username <username> -password <password> -rhceph-password <password>
```

### Example

```bash
./odfdr-installer -url api.cluster.example.com:6443 -username kubeadmin -password abc -rhceph-password xyz
```

### Flags

- `-url`: (Required) OpenShift API URL.
- `-username`: (Optional) OpenShift username (default: `kubeadmin`).
- `-password`: (Required) OpenShift password.
- `-rhceph-password`: (Required) RHCEPH repository password.

## Features

- Automatically logs into the specified OpenShift cluster.
- Adds CatalogSource and ImageContentSourcePolicy (ICSP) to your OpenShift cluster.
- Updates the pull secret with credentials from the RHCEPH repository.

## Configuration Files

- The tool embeds certain configuration files (`icsp.yaml`, `odf-catalogsource.yaml`) that define the necessary resources for the deployment.

## License

This project is licensed under the Apache License, Version 2.0. For more details, please refer to the [LICENSE](LICENSE) file.

## Contributing

Contributions are welcome! Please submit a pull request or open an issue to discuss changes.

## Troubleshooting

- Ensure all required commands are available in your PATH.
- Verify that you have the necessary permissions to access the OpenShift cluster.

For any additional questions or support, please open an issue in the repository.