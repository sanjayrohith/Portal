# Third-Party Software Licenses & Attributions

This project incorporates and links to open-source software packages under
permissive licenses. All dependencies have been verified for compliance with the
**Apache License 2.0**.

---

## License Summary Table

| Package | Version | License | Homepage / Source |
| :--- | :--- | :--- | :--- |
| `github.com/go-acme/lego/v4` | `v4.35.2` | MIT | https://github.com/go-acme/lego |
| `github.com/lib/pq` | `v1.12.3` | MIT | https://github.com/lib/pq |
| `github.com/prometheus/client_golang` | `v1.24.1` | Apache-2.0 | https://github.com/prometheus/client_golang |
| `github.com/spf13/cobra` | `v1.10.2` | Apache-2.0 | https://github.com/spf13/cobra |
| `github.com/spf13/pflag` | `v1.0.10` | BSD-3-Clause | https://github.com/spf13/pflag |
| `gopkg.in/yaml.v3` | `v3.0.1` | MIT / Apache-2.0 | https://github.com/go-yaml/yaml |
| `modernc.org/sqlite` | `v1.58.0` | BSD-3-Clause | https://gitlab.com/cznic/sqlite |
| `modernc.org/libc` | `v1.75.6` | BSD-3-Clause | https://gitlab.com/cznic/libc |
| `modernc.org/mathutil` | `v1.7.1` | BSD-3-Clause | https://gitlab.com/cznic/mathutil |
| `modernc.org/memory` | `v1.12.1` | BSD-3-Clause | https://gitlab.com/cznic/memory |
| `github.com/google/uuid` | `v1.6.0` | BSD-3-Clause | https://github.com/google/uuid |
| `github.com/mattn/go-isatty` | `v0.0.24` | MIT | https://github.com/mattn/go-isatty |
| `github.com/miekg/dns` | `v1.1.72` | BSD-3-Clause | https://github.com/miekg/dns |
| `golang.org/x/crypto` | `v0.54.0` | BSD-3-Clause | https://go.googlesource.com/crypto |
| `golang.org/x/net` | `v0.57.0` | BSD-3-Clause | https://go.googlesource.com/net |
| `golang.org/x/sys` | `v0.47.0` | BSD-3-Clause | https://go.googlesource.com/sys |
| `google.golang.org/protobuf` | `v1.36.11` | BSD-3-Clause | https://github.com/protocolbuffers/protobuf-go |

---

## License Texts

### Apache License 2.0
Used by: `github.com/prometheus/client_golang`, `github.com/spf13/cobra`

```
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
```

### MIT License
Used by: `github.com/go-acme/lego/v4`, `github.com/lib/pq`, `github.com/mattn/go-isatty`, `gopkg.in/yaml.v3`

```
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### BSD 3-Clause License
Used by: `modernc.org/sqlite`, `golang.org/x/*`, `github.com/google/uuid`, `github.com/miekg/dns`, `github.com/spf13/pflag`

```
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```
