# Public Apple trust anchors

Downloaded on 17 November 2026 from Apple's [PKI directory](https://www.apple.com/certificateauthority/).
These are public root certificates, not private keys.

| File               | Source                                                        | SHA-256                                                          |
| ------------------ | ------------------------------------------------------------- | ---------------------------------------------------------------- |
| AppleRootCA-G2.cer | https://www.apple.com/certificateauthority/AppleRootCA-G2.cer | c2b9b042dd57830e7d117dac55ac8ae19407d38e41d88f3215bc3a890444a050 |
| AppleRootCA-G3.cer | https://www.apple.com/certificateauthority/AppleRootCA-G3.cer | 63343abfb89a6a03ebb57e9b3f5fa7be7c4f5c756f3017b3a8c488c3653e9179 |

Review and update trust anchors explicitly if Apple rotates them. Never trust a root delivered in an incoming JWS solely because it appears in its `x5c` header.
