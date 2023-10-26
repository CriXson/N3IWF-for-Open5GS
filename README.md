## N3IWF for Open5GS 

The N3IWF in this repository is made compatible with the Open5GS core. The N3IWF is taken from the free5GC project.
For more information, please refer to [free5GC official site](https://free5gc.org/).

## Documentation
To start the Network Function.
```console
sudo ./bin/n3iwf
```
If you want to build the N3IWF
```console
cd ~/free5gc
make n3iwf
```
For a detailed manual, please reference to [Prerequisites](https://free5gc.org/guide/3-install-free5gc/#a-prerequisites).

Update the N3IWF config file.
1. Set the AMF IP
2. Set the ogstun ip as the IKEBindAddress (if Open5GS default ogstun ip is used, no changes needed)

For document, please reference to [Documentation](https://github.com/free5gc/free5gc/wiki).

## License

free5GC is now under [Apache 2.0](https://github.com/free5gc/free5gc/blob/master/LICENSE.txt) license.

