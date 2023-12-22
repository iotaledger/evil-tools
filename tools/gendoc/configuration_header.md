---
description: This section describes the configuration parameters and their types for the evil tools.
keywords:
- Evil Tools
- Evil Wallet
- Evil Spammer
- Configuration
- JSON
- Customize
- Config
- reference
---


# Configuration

evil tools use a JSON standard format as a config file. If you are unsure about JSON syntax, you can find more information in the [official JSON specs](https://www.json.org).

You can change the path of the config file by using the `-c` or `--config` argument while executing `evil-tools` executable.

For example:
```bash
evil-tools -c config_example.json
```

You can always get the most up-to-date description of the config parameters by running:

```bash
evil-tools -h --full
```

