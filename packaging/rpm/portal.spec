Name:           portal
Version:        1.0.0
Release:        1%{?dist}
Summary:        Open-source localhost tunneling tool with stable custom subdomains

License:        Apache-2.0
URL:            https://github.com/sanjayrohith/portal
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.22
ExclusiveArch:  x86_64 aarch64

%description
Portal is an open-source localhost tunneling tool designed for developers who
need permanent, stable subdomains for webhook development, mobile API testing,
and remote demos without relying on restrictive third-party services.

%prep
%autosetup

%build
export CGO_ENABLED=0
go build -trimpath -ldflags="-s -w" -o portal ./cmd/portal

%install
rm -rf $RPM_BUILD_ROOT
install -d %{buildroot}%{_bindir}
install -p -m 0755 portal %{buildroot}%{_bindir}/portal

install -d %{buildroot}%{_mandir}/man1
install -p -m 0644 docs/portal.1 %{buildroot}%{_mandir}/man1/portal.1

%files
%license LICENSE*
%doc README.md
%{_bindir}/portal
%{_mandir}/man1/portal.1*

%changelog
* Wed Nov 26 2025 Sanjay Rohith <sanjayrohith1802@gmail.com> - 1.0.0-1
- Initial release of portal package
