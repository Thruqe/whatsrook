use std::env;

#[derive(Debug, Clone, Default)]
pub enum ClientType {
    #[default]
    Chrome,
    Android,
    Ios,
}

impl ClientType {
    fn from_str(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "chrome" => Some(Self::Chrome),
            "android" => Some(Self::Android),
            "ios" => Some(Self::Ios),
            _ => None,
        }
    }
}

pub struct CliArgs {
    pub session: String,
    pub pair: Option<String>,
    pub port: Option<String>,
    pub auth_dir: Option<String>,
    pub qrcode: bool,
    pub logout: bool,
    pub debug: bool,
    pub client: ClientType,
}

impl CliArgs {
    pub fn parse() -> Self {
        let args: Vec<String> = env::args().collect();

        if args.contains(&"-h".to_string()) || args.contains(&"--help".to_string()) {
            println!(
                "Usage: zevBot --session <phone_number> [OPTIONS]

Options:
  --session <phone>     Phone number used to identify the session (required)
  --pair <phone>        Request a pair code for the given phone number
  --port <port>         Specify the HTTP/WebSocket port
  --auth-dir <path>     Directory to store session auth files (default: ./auth)
  --client <type>       Client type: chrome (default), android, ios
  --qrcode              Print the QR code to stdout for scanning
  --logout              Remove the session auth files and exit
  --debug               Enable debug logging
  -h, --help            Show this help message"
            );
            std::process::exit(0);
        }

        let get_value = |flag: &str| -> Option<String> {
            let index = args.iter().position(|a| a == flag)?;
            args.get(index + 1).cloned()
        };

        let session = get_value("--session").unwrap_or_else(|| {
            eprintln!("Error: --session <phone_number> is required. Run with -h for help.");
            std::process::exit(1);
        });

        let client = get_value("--client")
            .map(|s| {
                ClientType::from_str(&s).unwrap_or_else(|| {
                    eprintln!("Error: unknown --client '{s}'. Valid options: chrome, android, ios");
                    std::process::exit(1);
                })
            })
            .unwrap_or_default();

        Self {
            session,
            pair: get_value("--pair"),
            port: get_value("--port"),
            auth_dir: get_value("--auth-dir"),
            qrcode: args.contains(&"--qrcode".to_string()),
            logout: args.contains(&"--logout".to_string()),
            debug: args.contains(&"--debug".to_string()),
            client,
        }
    }
}