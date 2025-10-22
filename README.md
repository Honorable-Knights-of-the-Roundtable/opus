# Roundtable OPUS

Provide OPUS encoding for the [Roundtable](https://github.com/Honorable-Knights-of-the-Roundtable/Roundtable) VoIP application.

### Forked from [hraban/opus](https://github.com/hraban/opus)

The vast majority of this wrapper is from hraban's excellent repository. The additions made here are to facilitate building on Windows by means of embedding a DLL file into the executable.

This behavior is enabled by building with the `embedlibopusfile` tag, i.e.

```bash
# Do not embed the DLL, for use on Linux, MacOS
# where libopus is installed by a package manager
go build .  

# Embed the DLL, for use on Windows
go build -tags embedlibopusfile
```

## License

The licensing terms for the Go bindings are found in the LICENSE file. The
authors and copyright holders are listed in the AUTHORS file.

The copyright notice uses range notation to indicate all years in between are
subject to copyright, as well. This statement is necessary, apparently. For all
those nefarious actors ready to abuse a copyright notice with incorrect
notation, but thwarted by a mention in the README. Pfew!
