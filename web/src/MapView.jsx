import { MapContainer, TileLayer, CircleMarker, Tooltip } from 'react-leaflet';
import { useEffect, useState } from 'react';

function getColor(signalDbm) {
  if (signalDbm > -70) return 'green';
  if (signalDbm > -80) return 'orange';
  return 'red';
}

export default function MapView() {
  const [features, setFeatures] = useState([]);

  useEffect(() => {
    fetch(import.meta.env.BASE_URL + '/heatmap.json')
      .then(res => res.json())
      .then(data => {
        setFeatures(data.features || []);
      })
      .catch(error => {
        console.error('Error fetching heatmap data:', error);
      });
  }, []);

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      {/* <div style={{ backgroundColor: 'lightgray', padding: '10px' }}>
        Found {features.length} signal points
      </div> */}
      
      <div
        style={{
          flex: 1,
          position: 'relative',
          width: '100%'
        }}
      >
        <MapContainer
          center={[-19.9, -43.9]}
          zoom={12}
          style={{ height: '100%', width: '100%', position: 'absolute', top: 0, left: 0 }}
        >
          <TileLayer
            url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
            attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
          />
          
          {features.map((f, i) => {
            const [lon, lat] = f.geometry.coordinates;
            const { signalDbm, carrier } = f.properties;
            return (
              <CircleMarker
                key={i}
                center={[lat, lon]}
                radius={8}
                pathOptions={{ color: getColor(signalDbm), fillOpacity: 0.8, weight: 2 }}
              >
                <Tooltip>{carrier} | {signalDbm} dBm</Tooltip>
              </CircleMarker>
            );
          })}
        </MapContainer>
      </div>
    </div>
  );
}
