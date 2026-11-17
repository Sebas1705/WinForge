package main

// spec is one product WinForge tracks: where its dates come from and which catalog apps are an
// instance of it. This table is the fixed product list; adding software to the report means adding
// a line here, running the tool and reviewing the diff of catalogdata/eol.yml.
type spec struct {
	// Slug is the product's name on endoflife.date (https://endoflife.date/api/<slug>.json).
	Slug string
	// Vendor is the vendor's own lifecycle page, written as a comment so a reviewer can compare.
	Vendor string
	ID     string
	Name   string
	Scheme string
	// Apps are the catalog app ids that are an installed instance of the product.
	Apps []string
	// Follows are apps that always install the newest release ("latest") or newest LTS ("lts").
	Follows map[string]string
	// Install says which catalog app installs a given cycle.
	Install map[string]string
	// Only, when set, limits the data to these cycles (for products where only some lines are
	// something a catalog app installs and the date is clear).
	Only []string
}

// keepEndedSince drops cycles that ended before this day: nothing in the catalog installs them
// any more, and a very old version is reported as "unknown" rather than carrying hundreds of rows.
const keepEndedSince = "2022-01-01"

var specs = []spec{
	{
		Slug: "python", Vendor: "https://devguide.python.org/versions/", ID: "python", Name: "Python",
		Apps:    []string{"python-3-10", "python-3-11", "python-3-12", "python-3-13", "python-3-14"},
		Install: map[string]string{"3.10": "python-3-10", "3.11": "python-3-11", "3.12": "python-3-12", "3.13": "python-3-13", "3.14": "python-3-14"},
	},
	{
		Slug: "nodejs", Vendor: "https://github.com/nodejs/Release#release-schedule", ID: "nodejs", Name: "Node.js",
		Apps:    []string{"node-js", "nodejs-lts", "node-js-20", "node-js-22"},
		Follows: map[string]string{"nodejs-lts": "lts", "node-js": "latest"},
		Install: map[string]string{"20": "node-js-20", "22": "node-js-22"},
	},
	{
		Slug: "eclipse-temurin", Vendor: "https://adoptium.net/support/", ID: "temurin", Name: "Eclipse Temurin (Java)", Scheme: "java",
		Apps: []string{
			"temurin-17", "temurin-21", "temurin-25",
			"eclipse-temurin-jdk-with-hotspot-8", "eclipse-temurin-jdk-with-hotspot-11",
			"eclipse-temurin-jre-with-hotspot-8", "eclipse-temurin-jre-with-hotspot-11", "eclipse-temurin-jre-with-hotspot-17",
			"eclipse-temurin-jre-with-hotspot-21", "eclipse-temurin-jre-with-hotspot-25",
		},
		Install: map[string]string{
			"8": "eclipse-temurin-jdk-with-hotspot-8", "11": "eclipse-temurin-jdk-with-hotspot-11",
			"17": "temurin-17", "21": "temurin-21", "25": "temurin-25",
		},
	},
	{
		Slug: "microsoft-build-of-openjdk", Vendor: "https://learn.microsoft.com/java/openjdk/support", ID: "microsoft-openjdk", Name: "Microsoft Build of OpenJDK (Java)", Scheme: "java",
		Apps: []string{
			"microsoft-build-of-openjdk-with-hotspot-11", "microsoft-build-of-openjdk-with-hotspot-17",
			"microsoft-build-of-openjdk-with-hotspot-21", "microsoft-build-of-openjdk-with-hotspot-25",
		},
		Install: map[string]string{
			"11": "microsoft-build-of-openjdk-with-hotspot-11", "17": "microsoft-build-of-openjdk-with-hotspot-17",
			"21": "microsoft-build-of-openjdk-with-hotspot-21", "25": "microsoft-build-of-openjdk-with-hotspot-25",
		},
	},
	{
		Slug: "azul-zulu", Vendor: "https://www.azul.com/products/azul-support-roadmap/", ID: "azul-zulu", Name: "Azul Zulu (Java)", Scheme: "java",
		Apps:    []string{"azul-zulu-jdk-17", "azul-zulu-jdk-21"},
		Install: map[string]string{"17": "azul-zulu-jdk-17", "21": "azul-zulu-jdk-21"},
	},
	{
		Slug: "amazon-corretto", Vendor: "https://aws.amazon.com/corretto/faqs/", ID: "amazon-corretto", Name: "Amazon Corretto (Java)", Scheme: "java",
		Apps:    []string{"amazon-corretto-21"},
		Install: map[string]string{"21": "amazon-corretto-21"},
	},
	{
		Slug: "oracle-jdk", Vendor: "https://www.oracle.com/java/technologies/java-se-support-roadmap.html", ID: "oracle-jdk", Name: "Oracle JDK (Java)", Scheme: "java",
		Apps:    []string{"java-tm-se-development-kit-21", "java-tm-se-development-kit-25"},
		Install: map[string]string{"21": "java-tm-se-development-kit-21", "25": "java-tm-se-development-kit-25"},
	},
	{
		Slug: "dotnet", Vendor: "https://dotnet.microsoft.com/platform/support/policy/dotnet-core", ID: "dotnet", Name: ".NET",
		Apps: []string{
			"dotnet-sdk-8", "dotnet-sdk-9", "dotnet-sdk-10",
			"microsoft-net-runtime-6-0", "microsoft-net-runtime-8-0", "microsoft-net-runtime-9-0", "microsoft-net-runtime-10-0",
			"microsoft-asp-net-core-runtime-6-0", "microsoft-asp-net-core-runtime-8-0", "microsoft-asp-net-core-runtime-9-0", "microsoft-asp-net-core-runtime-10-0",
			"microsoft-asp-net-core-hosting-bundle-8-0", "microsoft-asp-net-core-hosting-bundle-9-0",
			"microsoft-net-windows-desktop-runtime-6-0", "microsoft-net-windows-desktop-runtime-7-0", "microsoft-net-windows-desktop-runtime-8-0",
			"microsoft-net-windows-desktop-runtime-9-0", "microsoft-net-windows-desktop-runtime-10-0",
		},
		Install: map[string]string{"8": "dotnet-sdk-8", "9": "dotnet-sdk-9", "10": "dotnet-sdk-10"},
	},
	{
		Slug: "powershell", Vendor: "https://learn.microsoft.com/powershell/scripting/install/powershell-support-lifecycle", ID: "powershell", Name: "PowerShell 7",
		Apps:    []string{"powershell"},
		Follows: map[string]string{"powershell": "latest"},
	},
	{
		Slug: "go", Vendor: "https://go.dev/doc/devel/release", ID: "go", Name: "Go",
		Apps:    []string{"go"},
		Follows: map[string]string{"go": "latest"},
	},
	{
		Slug: "php", Vendor: "https://www.php.net/supported-versions.php", ID: "php", Name: "PHP",
		Apps:    []string{"php-8-4"},
		Install: map[string]string{"8.4": "php-8-4"},
	},
	{
		Slug: "ruby", Vendor: "https://www.ruby-lang.org/en/downloads/branches/", ID: "ruby", Name: "Ruby",
		Apps:    []string{"ruby-3-4", "ruby-3-4-with-msys2"},
		Install: map[string]string{"3.4": "ruby-3-4"},
	},
	{
		Slug: "mysql", ID: "mysql", Name: "MySQL",
		Apps: []string{"mysql"},
	},
	{
		Slug: "mariadb", Vendor: "https://mariadb.org/about/#maintenance-policy", ID: "mariadb", Name: "MariaDB",
		Apps: []string{"mariadb"},
	},
	{
		Slug: "postgresql", Vendor: "https://www.postgresql.org/support/versioning/", ID: "postgresql", Name: "PostgreSQL",
		Apps:    []string{"postgresql-15", "postgresql-16", "postgresql-17"},
		Install: map[string]string{"15": "postgresql-15", "16": "postgresql-16", "17": "postgresql-17"},
	},
	{
		Slug: "mongodb", Vendor: "https://www.mongodb.com/legal/support-policy/lifecycles", ID: "mongodb", Name: "MongoDB Server",
		Apps: []string{"mongodb"},
	},
	{
		// Only 2022 is something the catalog installs. The date is the end of extended support,
		// which is when Microsoft stops security updates.
		Slug: "mssqlserver", Vendor: "https://learn.microsoft.com/lifecycle/products/sql-server-2022", ID: "sql-server-2022", Name: "SQL Server 2022",
		Apps: []string{"microsoft-sql-server-2022-express", "microsoft-sql-server-2022-developer"},
		Only: []string{"16.0"},
	},
	{
		// Visual Studio is left to its single servicing line: 16.11 is the one 2019 release that
		// is supported (until the end of extended support). Other 16.x minors are not listed, so
		// they show as "unknown" rather than with a date that is hard to state.
		Slug: "visual-studio", Vendor: "https://learn.microsoft.com/lifecycle/products/visual-studio-2019", ID: "visual-studio-2019", Name: "Visual Studio 2019",
		Apps: []string{"visual-studio-buildtools-2019"},
		Only: []string{"16.11"},
	},
}
