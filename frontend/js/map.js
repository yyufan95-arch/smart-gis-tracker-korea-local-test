let map;
const pathCoords = [
  [35.9119467, 127.0659829], // A 문화관
  [35.9121794, 127.0665127], // B 정궁관
  [35.9126749, 127.0685961], // C 체육관 
  [35.9117154, 127.0666587]  // D 학생관
];

function initMap() {
  console.log("Leaflet 지도 초기화를 시작합니다...");

  // 1. 지도 초기화 및 중심 좌표 설정
  map = L.map('map').setView(pathCoords[0], 16);

  // 2. 지도 타일 로드 (오픈스트리트맵, 별도 키 불필요)
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '© OpenStreetMap contributors'
  }).addTo(map);

  // 3. 지점 간 붉은 연결선 그리기
  L.polyline(pathCoords, {color: 'red', weight: 3}).addTo(map);

  // 4. A, B, C, D 지점 마커 표시
  const labels = ['A', 'B', 'C', 'D'];
  pathCoords.forEach((coord, index) => {
      L.marker(coord).addTo(map)
        .bindPopup("위치: " + labels[index])
        .bindTooltip(labels[index], {permanent: true, direction: 'right'});
  });
}

// 페이지 로드 시 지도 실행
window.addEventListener('DOMContentLoaded', initMap);
