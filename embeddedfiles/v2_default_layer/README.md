# Schoolyear base layer

This layer is built-in and prepares the Schoolyear base image.
It should be included in every image used with Schoolyear AVD.
The AVD CLI includes this layer in every bundle by default.

- Network:
  - Whitelist windows hosts
  - Configure system proxy
  - Configure proxy domain (write to hosts file !!!)
  - Lockdown firewall
  - Configure user proxy
- Setup script: sessionhost setup script
- VDI browser
  - auto-update browser script (included in deployment scripts?) -> move to deployment template folder & adapt CI pipeline
- VDOT
  - Reboot (!?)


- clean: DONE
- Common config: DONE
- network
  - whitelist hosts: DONE
  - setup system proxy (setup script) -> fix writing to the host file: DONE
  - lockdown firewall (setup script): DONE
  - configure user proxy (user script): DONE
- scripts setup: sessionhost setup script -> implement in template
- VDI browser
  - auto-update browser script -> move folder & implement in template
- VDOT: DONE
  - Reboot (!?) -> not doing it