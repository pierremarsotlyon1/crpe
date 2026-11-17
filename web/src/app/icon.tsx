import { ImageResponse } from "next/og";
export const size = { width: 64, height: 64 };
export const contentType = "image/png";
export default function Icon() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        borderRadius: 14,
        background: "#173c35",
        color: "#faf9f5",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        fontSize: 48,
        fontWeight: 700,
        letterSpacing: -5,
      }}
    >
      m<span style={{ color: "#d6bb74" }}>.</span>
    </div>,
    size,
  );
}
