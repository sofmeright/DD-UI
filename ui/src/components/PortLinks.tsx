// ui/src/components/PortLinks.tsx
import { ExternalLink } from "lucide-react";

interface PortInfo {
  ip: string;
  publicPort: number | string;
  privatePort: number | string;
  type: string;
}

function parsePortsData(ports: any): PortInfo[] {
  const arr: any[] =
    Array.isArray(ports) ? ports :
      (ports && Array.isArray(ports.ports)) ? ports.ports : [];
  
  const portInfos: PortInfo[] = [];
  const seen = new Set<string>(); // Track unique public:private/type combinations
  
  for (const p of arr) {
    const ip = p.IP || p.Ip || p.ip || "";
    const pub = p.PublicPort ?? p.publicPort;
    const priv = p.PrivatePort ?? p.privatePort;
    const typ = (p.Type ?? p.type ?? "").toString().toLowerCase() || "tcp";
    
    if (pub && priv) {
      // Create unique key for public:private/type combination
      const key = `${pub}:${priv}/${typ}`;
      
      // Skip if we've already seen this port mapping
      if (seen.has(key)) {
        continue;
      }
      
      seen.add(key);
      portInfos.push({
        ip: ip || "0.0.0.0",
        publicPort: pub,
        privatePort: priv,
        type: typ
      });
    }
  }
  return portInfos;
}

function getProtocolForPort(publicPort: number | string, privatePort: number | string): string {
  const publicPortNum = typeof publicPort === 'string' ? parseInt(publicPort, 10) : publicPort;
  const privatePortNum = typeof privatePort === 'string' ? parseInt(privatePort, 10) : privatePort;
  
  // Check both public and private ports for HTTP/HTTPS decision
  // HTTP only if either port is 80
  if (publicPortNum === 80 || privatePortNum === 80) return "http";
  
  // HTTPS if either port is 443
  if (publicPortNum === 443 || privatePortNum === 443) return "https";
  
  // Common HTTP ports - check both public and private
  const httpPorts = [8080, 8000, 3000, 4000, 5000, 9000];
  if (httpPorts.includes(publicPortNum) || httpPorts.includes(privatePortNum)) return "http";
  
  // Common HTTPS ports - check both public and private
  const httpsPorts = [8443, 9443, 3443, 4443, 5443];
  if (httpsPorts.includes(publicPortNum) || httpsPorts.includes(privatePortNum)) return "https";
  
  // Default to HTTPS for security
  return "https";
}

function buildServiceUrl(hostAddress: string, publicPort: number | string, privatePort: number | string): string {
  const protocol = getProtocolForPort(publicPort, privatePort);
  const portNum = typeof publicPort === 'string' ? parseInt(publicPort, 10) : publicPort;
  
  // Don't show port for standard ports
  if ((protocol === "https" && portNum === 443) || (protocol === "http" && portNum === 80)) {
    return `${protocol}://${hostAddress}`;
  }
  
  return `${protocol}://${hostAddress}:${portNum}`;
}

export default function PortLinks({ 
  ports, 
  hostAddress,
  className = "" 
}: { 
  ports: any; 
  hostAddress: string;
  className?: string;
}) {
  const portInfos = parsePortsData(ports);
  
  if (portInfos.length === 0) {
    return <div className={className}>—</div>;
  }

  return (
    <div className={`space-y-1 ${className}`}>
      {portInfos.map((port, i) => {
        const serviceUrl = buildServiceUrl(hostAddress, port.publicPort, port.privatePort);
        const displayText = `${port.publicPort} → ${port.privatePort}/${port.type}`;
        
        return (
          <div key={i} className="flex items-center gap-2">
            <a 
              href={serviceUrl}
              target="_blank"
              rel="noopener noreferrer" 
              className="flex items-center gap-1 text-blue-400 hover:text-blue-300 hover:underline text-sm"
              title={`Open ${serviceUrl}`}
            >
              <ExternalLink className="h-3 w-3" />
              {displayText}
            </a>
          </div>
        );
      })}
    </div>
  );
}